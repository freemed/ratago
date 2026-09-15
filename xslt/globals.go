package xslt

import (
	"regexp"
	"sort"
	"strings"

	"github.com/freemed/gokogiri/xml"
)

// This file makes the evaluation of global variables/parameters — and the
// namespace declarations copied onto the output root — independent of Go's
// randomised map iteration order.
//
// Why it matters: the same binary, input and stylesheet used to produce
// different output on consecutive runs (measured on resources/xsl/4010_837p.xsl:
// 15286, 15287, 15288 and 15289 bytes for outputs whose only difference was a
// single `<hl>` value). Two defects combined:
//
//  1. Global variables were evaluated by ranging over the Stylesheet.Variables
//     map. Because a global may reference another global (the REMITT
//     stylesheets compute `<xsl:variable name="patientoffset"
//     select="count($insuredall)+$insuredoffset"/>`), whichever variable was
//     visited first saw its dependency as "not found" — the XPath resolver
//     returns an empty node-set in that case, so count() reported 0 and the
//     addition produced 0/NaN.
//  2. That wrong value was then frozen: the retry passes only re-evaluated
//     variables whose Value was still nil, and a numeric result is never nil.
//
// Global variables are now evaluated in dependency order with declaration
// (document) order as the tie-break. That is the order libxslt effectively uses
// (it resolves a global variable reference by evaluating the referenced global
// on demand), so the byte output matches xsltproc as well as being stable.

// variableRefRe matches a $variable reference as it appears in an XPath
// expression, including an optional namespace prefix.
var variableRefRe = regexp.MustCompile(`\$([\pL_][\pL\pN_.\-]*(?::[\pL_][\pL\pN_.\-]*)?)`)

// stringSliceContains reports whether s contains v.
func stringSliceContains(s []string, v string) bool {
	for _, item := range s {
		if item == v {
			return true
		}
	}
	return false
}

// globalVariableNames returns the stylesheet's global variables and parameters
// in declaration order. Variables registered without going through
// RegisterGlobalVariable (hand-built Stylesheets in tests) are appended in
// sorted order so the result is always deterministic.
func (style *Stylesheet) globalVariableNames() []string {
	names := make([]string, 0, len(style.Variables))
	seen := make(map[string]bool, len(style.Variables))
	for _, name := range style.GlobalVariableOrder {
		if _, ok := style.Variables[name]; !ok || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	var rest []string
	for name := range style.Variables {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append(names, rest...)
}

// namespaceURIs returns the stylesheet's namespace URIs in declaration order,
// followed by any URI that reached NamespaceMapping some other way in sorted
// order. Never map order.
func (style *Stylesheet) namespaceURIs() []string {
	uris := make([]string, 0, len(style.NamespaceMapping))
	seen := make(map[string]bool, len(style.NamespaceMapping))
	for _, uri := range style.NamespaceOrder {
		if _, ok := style.NamespaceMapping[uri]; !ok || seen[uri] {
			continue
		}
		seen[uri] = true
		uris = append(uris, uri)
	}
	var rest []string
	for uri := range style.NamespaceMapping {
		if !seen[uri] {
			rest = append(rest, uri)
		}
	}
	sort.Strings(rest)
	return append(uris, rest...)
}

// namespaceForPrefix returns the URI the stylesheet binds to prefix. When
// several URIs share a prefix the first one in declaration order wins; the
// previous map-range loop returned whichever the hash order happened to hit
// first.
func (style *Stylesheet) namespaceForPrefix(prefix string) (uri string, ok bool) {
	for _, u := range style.namespaceURIs() {
		if style.NamespaceMapping[u] == prefix {
			return u, true
		}
	}
	return "", false
}

// variableRefsIn appends the distinct $variable names referenced by expr to
// *refs, in first-seen order.
func variableRefsIn(expr string, refs *[]string, seen map[string]bool) {
	for _, m := range variableRefRe.FindAllStringSubmatch(expr, -1) {
		name := m[1]
		if i := strings.LastIndexByte(name, ':'); i >= 0 {
			name = name[i+1:]
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		*refs = append(*refs, name)
	}
}

// collectVariableRefs gathers $variable references from every attribute of node
// and its descendants. xsl:call-template is followed into the named template's
// body, because a global variable whose body calls a template depends on every
// global that template reads.
func (style *Stylesheet) collectVariableRefs(node xml.Node, refs *[]string, seen map[string]bool, visitedTemplates map[string]bool) {
	if node == nil {
		return
	}
	for _, attr := range node.AttributeList() {
		variableRefsIn(attr.Value(), refs, seen)
	}
	if IsXsltName(node, "call-template") {
		name := node.Attr("name")
		if name != "" && !visitedTemplates[name] {
			visitedTemplates[name] = true
			if t, ok := style.NamedTemplates[name]; ok && t.Node != nil {
				style.collectVariableRefs(t.Node, refs, seen, visitedTemplates)
			}
		}
	}
	for cur := node.FirstChild(); cur != nil; cur = cur.NextSibling() {
		style.collectVariableRefs(cur, refs, seen, visitedTemplates)
	}
}

// globalVariableDependencies returns the global variables that name must be
// evaluated after, in first-seen order. References to anything that is not a
// global variable (a template parameter, a local variable, a variable of an
// imported stylesheet that was not registered here) are dropped: they can never
// block a global's evaluation.
func (style *Stylesheet) globalVariableDependencies(name string) []string {
	v, ok := style.Variables[name]
	if !ok || v == nil {
		return nil
	}
	var refs []string
	seen := make(map[string]bool)
	if sel := v.Node.Attr("select"); sel != "" {
		// A select expression is evaluated in one shot: only the names it
		// mentions (not the bodies of templates it may call) matter.
		variableRefsIn(sel, &refs, seen)
	} else {
		style.collectVariableRefs(v.Node, &refs, seen, make(map[string]bool))
	}
	deps := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref == name {
			continue // self-reference: XSLT error, nothing to order against
		}
		if _, ok := style.Variables[ref]; ok {
			deps = append(deps, ref)
		}
	}
	return deps
}

// evaluateGlobalVariables evaluates every global variable and parameter before
// the source tree is processed. A global whose dependencies have already been
// evaluated is evaluated as soon as it comes up in declaration order; if none
// of the remaining globals is ready (a dependency cycle, or a dependency the
// static scan could not see) the rest are evaluated in declaration order so
// behaviour degrades to the previous single-pass case instead of dropping
// variables.
func (style *Stylesheet) evaluateGlobalVariables(root xml.Node, context *ExecutionContext) {
	order := style.globalVariableNames()

	deps := make(map[string][]string, len(order))
	evaluated := make(map[string]bool, len(order))
	pending := make([]string, 0, len(order))
	for _, name := range order {
		v := style.Variables[name]
		if v == nil {
			continue
		}
		if v.Value != nil {
			// Already has a value: supplied by the caller as a parameter
			// (bound before this call), or a leftover from a previous run.
			evaluated[name] = true
			continue
		}
		deps[name] = style.globalVariableDependencies(name)
		pending = append(pending, name)
	}

	evaluate := func(name string) {
		if v := style.Variables[name]; v != nil {
			v.Apply(root, context)
		}
		evaluated[name] = true
	}

	for len(pending) > 0 {
		var remaining []string
		progressed := false
		for _, name := range pending {
			ready := true
			for _, dep := range deps[name] {
				if !evaluated[dep] {
					ready = false
					break
				}
			}
			if !ready {
				remaining = append(remaining, name)
				continue
			}
			evaluate(name)
			progressed = true
		}
		if !progressed {
			for _, name := range remaining {
				evaluate(name)
			}
			return
		}
		pending = remaining
	}
}
