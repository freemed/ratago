package xslt

import (
	"container/list"
	"errors"
	"fmt"
	"github.com/freemed/gokogiri/xml"
	"github.com/freemed/gokogiri/xpath"
	antchfx "github.com/freemed/xpath"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"
)

// ExecutionContext is passed to XSLT instructions during processing.
type ExecutionContext struct {
	Style           *Stylesheet                 // The master stylesheet
	Output          xml.Document                // The output document
	Source          xml.Document                // The source input document
	OutputNode      xml.Node                    // The current output node
	Current         xml.Node                    // The node that will be returned for "current()"
	XPathContext    *xpath.XPath                //the XPath context
	Mode            string                      //The current template mode
	Stack           list.List                   //stack used for scoping local variables
	InputDocuments  map[string]*xml.XmlDocument //additional input documents via document()
	CurrentTemplate *Template                   //the template currently being applied
}

func (context *ExecutionContext) EvalXPath(xmlNode xml.Node, data interface{}) (result interface{}, err error) {
	switch data := data.(type) {
	case string:
		data = context.contextualizeXPath(data)
		// Try standard compilation first. If the expression contains
		// $variable references or potential extension functions (namespaced
		// calls like set:distinct), try compilation with resolvers.
		var xpathExpr *xpath.Expression
		if strings.Contains(data, "$") || strings.Contains(data, ":") {
			vr, fr := context.xpathResolvers()
			xpathExpr = xpath.CompileWithResolvers(data, nil, vr, fr)
		}
		if xpathExpr == nil {
			xpathExpr = xpath.Compile(data)
		}
		// Plain compilation also fails for calls to XSLT-only functions that
		// the XPath engine does not implement as built-ins (current(),
		// format-number(), generate-id(), key(), ...), so a failing plain
		// compile is not the end of the road: retry with the XSLT
		// variable/function resolvers attached before giving up.
		if xpathExpr == nil {
			vr, fr := context.xpathResolvers()
			xpathExpr = xpath.CompileWithResolvers(data, nil, vr, fr)
		}
		if xpathExpr != nil {
			defer xpathExpr.Free()
			result, err = context.EvalXPath(xmlNode, xpathExpr)
		} else {
			err = errors.New("cannot compile xpath: " + data)
		}
	case []byte:
		result, err = context.EvalXPath(xmlNode, string(data))
	case *xpath.Expression:
		// An expression containing position()/last() at its top level must be
		// evaluated with the XSLT context position, which the XPath engine
		// cannot know (see contextualizeXPath). Recompile the rewritten text.
		if src := data.String(); src != "" {
			if rewritten := context.contextualizeXPath(src); rewritten != src {
				return context.EvalXPath(xmlNode, rewritten)
			}
		}
		xpathCtx := context.XPathContext
		xpathCtx.SetResolver(context)
		err := xpathCtx.Evaluate(xmlNode.NodePtr(), data)
		if err != nil {
			return nil, err
		}
		rt := xpathCtx.ReturnType()
		switch rt {
		case xpath.XPATH_NODESET, xpath.XPATH_XSLT_TREE:
			nodePtrs, err := xpathCtx.ResultAsNodeset()
			if err != nil {
				return nil, err
			}
			var output []xml.Node
			for _, nodePtr := range nodePtrs {
				output = append(output, context.nodeFromResult(nodePtr, xmlNode))
			}
			result = output
		case xpath.XPATH_NUMBER:
			result, err = xpathCtx.ResultAsNumber()
		case xpath.XPATH_BOOLEAN:
			result, err = xpathCtx.ResultAsBoolean()
		default:
			result, err = xpathCtx.ResultAsString()
		}
	default:
		err = errors.New("Strange type passed to ExecutionContext.EvalXPath")
	}
	return
}

// Register the namespaces in scope with libxml2 so that XPaths with namespaces
// are resolved correctly.
//
// libxml2 probably already makes this info available
func (context *ExecutionContext) RegisterXPathNamespaces(node xml.Node) (err error) {
	seen := make(map[string]bool)
	for n := node; n != nil; n = n.Parent() {
		for _, decl := range n.DeclaredNamespaces() {
			alreadySeen, _ := seen[decl.Prefix]
			if !alreadySeen {
				context.XPathContext.RegisterNamespace(decl.Prefix, decl.Uri)
				seen[decl.Prefix] = true
			}
		}
	}
	return
}

// Attempt to map a prefix to a URI.
func (context *ExecutionContext) LookupNamespace(prefix string, node xml.Node) (uri string) {
	//if given a context node, see if the prefix is in scope
	if node != nil {
		for n := node; n != nil; n = n.Parent() {
			for _, decl := range n.DeclaredNamespaces() {
				if decl.Prefix == prefix {
					return decl.Uri
				}
			}
		}
	}

	//if no context node, or prefix not found in node scope, check the stylesheet map
	for href, pre := range context.Style.NamespaceMapping {
		if pre == prefix {
			return href
		}
	}
	return
}

func (context *ExecutionContext) EvalXPathAsNodeset(xmlNode xml.Node, data interface{}) (result xml.Nodeset, err error) {
	_, err = context.EvalXPath(xmlNode, data)
	if err != nil {
		return nil, err
	}
	nodePtrs, err := context.XPathContext.ResultAsNodeset()
	if err != nil {
		return nil, err
	}
	var output xml.Nodeset
	for _, nodePtr := range nodePtrs {
		output = append(output, context.nodeFromResult(nodePtr, xmlNode))
	}
	result = output
	return
}

// CompileSelect compiles an XPath @select/@test expression into a form
// suitable for EvalXPath / EvalXPathAsNodeset / EvalXPathAsString.
//
// xpath.Compile() runs without variable or function resolvers, so any
// expression containing a $variable reference (or an unknown extension
// function) fails to compile and returns nil. Instructions that held the
// pre-compiled expression then evaluated a nil expression, which yields an
// empty result — silently dropping the whole selection (this is what broke
// `for-each select="$nodeset"`). When the plain compilation fails we hand
// back the raw string instead: EvalXPath recompiles it with the XSLT
// variable/function resolvers attached, so the variable is resolved at
// evaluation time.
func (context *ExecutionContext) CompileSelect(expr string) interface{} {
	if e := xpath.Compile(expr); e != nil {
		return e
	}
	return expr
}

// nodeFromResult converts an XPath result node pointer to an xml.Node,
// handling regular InternalNode pointers, AttrNode results, and
// generic NodeNavigator implementations (e.g. scalarNavigator).
func (context *ExecutionContext) nodeFromResult(nodePtr interface{}, refNode xml.Node) xml.Node {
	switch n := nodePtr.(type) {
	case *xml.InternalNode:
		return xml.NewNode(n, refNode.MyDocument())
	case *xpath.AttrNode:
		inner := &xml.InternalNode{
			Typ:     xml.XML_ATTRIBUTE_NODE,
			Name:    n.Name_,
			Content: n.Value_,
			Valid:   true,
		}
		if n.Prefix_ != "" || n.NamespaceURI_ != "" {
			inner.Ns = &xml.InternalNs{Prefix: n.Prefix_, Href: n.NamespaceURI_}
		}
		if refNode != nil {
			if parentInner, ok := refNode.NodePtr().(*xml.InternalNode); ok {
				inner.Parent = parentInner
			}
		}
		return xml.NewNode(inner, refNode.MyDocument())
	case antchfx.NodeNavigator:
		// Generic navigator (e.g. scalarNavigator): create a text node
		inner := &xml.InternalNode{
			Typ:     xml.XML_TEXT_NODE,
			Content: n.Value(),
			Valid:   true,
		}
		if refNode != nil {
			if parentInner, ok := refNode.NodePtr().(*xml.InternalNode); ok {
				inner.Parent = parentInner
			}
		}
		return xml.NewNode(inner, refNode.MyDocument())
	}
	return nil
}

func (context *ExecutionContext) EvalXPathAsBoolean(xmlNode xml.Node, data interface{}) (result bool) {
	_, err := context.EvalXPath(xmlNode, data)
	if err != nil {
		return false
	}
	result, _ = context.XPathContext.ResultAsBoolean()
	return
}

func (context *ExecutionContext) EvalXPathAsString(xmlNode xml.Node, data interface{}) (result string, err error) {
	_, err = context.EvalXPath(xmlNode, data)
	if err != nil {
		return
	}
	result, err = context.XPathContext.ResultAsString()
	return
}

// ChildrenOf returns the node children, ignoring any whitespace-only text nodes that
// are stripped by strip-space or xml:space
func (context *ExecutionContext) ChildrenOf(node xml.Node) (children []xml.Node) {
	if node == nil {
		return
	}
	for cur := node.FirstChild(); cur != nil; cur = cur.NextSibling() {
		//don't count stripped nodes
		if context.ShouldStrip(cur) {
			continue
		}
		children = append(children, cur)
	}
	return
}

// ShouldStrip evaluates the strip-space, preserve-space, and xml:space rules
// and returns true if a node is a whitespace-only text node that should
// be stripped.
func (context *ExecutionContext) ShouldStrip(xmlNode xml.Node) bool {
	if xmlNode.NodeType() != xml.XML_TEXT_NODE {
		return false
	}
	if !IsBlank(xmlNode) {
		return false
	}

	// Check for xml:space="preserve" on any ancestor
	for anc := xmlNode.Parent(); anc != nil; anc = anc.Parent() {
		if anc.NodeType() == xml.XML_ELEMENT_NODE {
			space := anc.Attr("space")
			xmlns := anc.Namespace()
			if xmlns == XML_NAMESPACE && space == "preserve" {
				return false
			}
		}
	}

	//do we have a match in strip-space?
	elem := xmlNode.Parent().Name()
	ns := xmlNode.Parent().Namespace()
	for _, pat := range context.Style.StripSpace {
		if pat == elem {
			return true
		}
		if pat == "*" {
			return true
		}
		if strings.Contains(pat, ":") {
			uri, name := context.ResolveQName(pat)
			if uri == ns {
				if name == elem || name == "*" {
					return true
				}
			}
		}
	}
	//do we have a match in preserve-space?
	// Preserve-space beats strip-space for equal specificity (spec rule)
	for _, pat := range context.Style.PreserveSpace {
		if pat == elem {
			return false
		}
		if pat == "*" {
			return false
		}
		if strings.Contains(pat, ":") {
			uri, name := context.ResolveQName(pat)
			if uri == ns {
				if name == elem || name == "*" {
					return false
				}
			}
		}
	}
	//return a value
	return false
}

func (context *ExecutionContext) ResolveQName(qname string) (ns, name string) {
	if !strings.Contains(qname, ":") {
		// no prefix: use the default namespace from the current context
		name = qname
		if context.Current != nil {
			ns = context.DefaultNamespace(context.Current)
		}
		return
	}
	parts := strings.Split(qname, ":")
	for uri, prefix := range context.Style.NamespaceMapping {
		if prefix == parts[0] {
			return uri, parts[1]
		}
	}
	// also try resolving through in-scope namespaces
	if context.Current != nil {
		uri := context.LookupNamespace(parts[0], context.Current)
		if uri != "" {
			return uri, parts[1]
		}
	}
	return
}

func (context *ExecutionContext) UseCDataSection(node xml.Node) bool {
	if node.NodeType() != xml.XML_ELEMENT_NODE {
		return false
	}
	name := node.Name()
	ns := node.Namespace()
	for _, el := range context.Style.CDataElements {
		if el == name {
			return true
		}
		uri, elname := context.ResolveQName(el)
		if uri == ns && name == elname {
			return true
		}
	}
	return false
}

func (context *ExecutionContext) ResolveVariable(name, ns string) (ret interface{}) {
	v := context.FindVariable(name, ns)

	if v == nil {
		return
	}

	switch val := v.Value.(type) {
	case xml.Nodeset:
		return val.ToXPathNodeset()
	case []xml.Node:
		nodeset := xml.Nodeset(val)
		return nodeset.ToXPathNodeset()
	default:
		return val
	}
}

func (context *ExecutionContext) FindVariable(name, ns string) (ret *Variable) {
	//consult local vars
	//consult local params
	v := context.LookupLocalVariable(name, ns)
	if v != nil {
		return v
	}
	//consult global vars (ss)
	//consult global params (ss)
	v, ok := context.Style.Variables[name]
	if ok {
		return v
	}
	return nil
}

// nodeSetNavigators converts an XSLT node-set (a slice of gokogiri nodes) into
// the representation the XPath engine understands as a node-set: ALWAYS a
// []antchfx.NodeNavigator, even for zero or one node.
//
// Two properties matter here:
//   - An empty node-set must stay a node-set. Returning "" (a string) made the
//     engine wrap it in a synthetic single-value navigator, so
//     count($empty) == 1 and xsl:for-each iterated once instead of not at all.
//   - A 1-node set and an N-node set must take the same code path, so they can
//     only differ in size.
func nodeSetNavigators(nodes []xml.Node) []antchfx.NodeNavigator {
	navs := make([]antchfx.NodeNavigator, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if adapter, ok := n.NodePtr().(xpath.NodeAdapter); ok {
			navs = append(navs, xpath.NewNavigator(adapter))
		}
	}
	return navs
}

// nodeSetNavigatorsFromPointers is nodeSetNavigators for raw gokogiri
// InternalNode pointers (the shape EXSLT node-set functions return).
func nodeSetNavigatorsFromPointers(ptrs []unsafe.Pointer) []antchfx.NodeNavigator {
	navs := make([]antchfx.NodeNavigator, 0, len(ptrs))
	for _, p := range ptrs {
		if p == nil {
			continue
		}
		navs = append(navs, xpath.NewNavigator((*xml.InternalNode)(p)))
	}
	return navs
}

// ResolveXPathVariable implements antchfx.VariableResolver for the XPath engine.
// It resolves $variable references in XPath expressions by looking up the
// variable in local scope, then global scope. Returns the value in a form
// compatible with antchfx/xpath: string, float64, bool, or a node-set
// ([]antchfx.NodeNavigator).
func (context *ExecutionContext) ResolveXPathVariable(prefix, name string) (interface{}, error) {
	ns := ""
	if prefix != "" {
		ns = context.LookupNamespace(prefix, context.Current)
	}
	v := context.FindVariable(name, ns)
	if v == nil || v.Value == nil {
		return "", fmt.Errorf("variable $%s not found", name)
	}
	switch val := v.Value.(type) {
	case string:
		return val, nil
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case bool:
		return val, nil
	case xml.Nodeset:
		return nodeSetNavigators(val), nil
	case []xml.Node:
		return nodeSetNavigators(val), nil
	case []unsafe.Pointer:
		return nodeSetNavigatorsFromPointers(val), nil
	case []interface{}:
		var nodes []xml.Node
		for _, item := range val {
			switch i := item.(type) {
			case *xml.InternalNode:
				nodes = append(nodes, xml.NewNode(i, nil))
			case unsafe.Pointer:
				nodes = append(nodes, xml.NewNode((*xml.InternalNode)(i), nil))
			}
		}
		return nodeSetNavigators(nodes), nil
	default:
		return fmt.Sprintf("%v", val), nil
	}
}

// ResolveXPathFunction implements antchfx.FunctionResolver for the XPath engine.
// It resolves unknown function calls by looking up the function in the XSLT
// function registry and calling it with the provided arguments.
func (context *ExecutionContext) ResolveXPathFunction(prefix, name string, args []interface{}) (interface{}, error) {
	ns := ""
	if prefix != "" {
		ns = context.LookupNamespace(prefix, context.Current)
	}
	if !context.IsFunctionRegistered(name, ns) {
		return nil, fmt.Errorf("function %s not registered", name)
	}
	fn := context.ResolveFunction(name, ns)
	if fn == nil {
		return nil, fmt.Errorf("function %s resolver is nil", name)
	}
	// Normalize arguments: convert antchfx NodeNavigator types to
	// gokogiri *xml.InternalNode which XSLT functions expect.
	normalized := make([]interface{}, len(args))
	for i, arg := range args {
		normalized[i] = context.normalizeXPathArg(arg)
	}
	result := fn(context, normalized)
	// Normalize the return value too: convert XSLT function results
	// (e.g. []unsafe.Pointer from nodeset functions) back to types
	// that the XPath engine can iterate.
	return context.normalizeXPathReturn(result), nil
}

// normalizeXPathReturn converts XSLT function return values (e.g.
// []unsafe.Pointer, xml.Nodeset) into types that the XPath engine can
// iterate via Select (NodeNavigator or []NodeNavigator).
func (context *ExecutionContext) normalizeXPathReturn(result interface{}) interface{} {
	switch v := result.(type) {
	case []unsafe.Pointer:
		var navs []antchfx.NodeNavigator
		for _, p := range v {
			inner := (*xml.InternalNode)(p)
			navs = append(navs, xpath.NewNavigator(inner))
		}
		return navs
	case xml.Nodeset:
		var navs []antchfx.NodeNavigator
		for _, n := range v {
			navs = append(navs, xpath.NewNavigator(n.NodePtr().(xpath.NodeAdapter)))
		}
		return navs
	case []xml.Node:
		var navs []antchfx.NodeNavigator
		for _, n := range v {
			navs = append(navs, xpath.NewNavigator(n.NodePtr().(xpath.NodeAdapter)))
		}
		return navs
	case []interface{}:
		var navs []antchfx.NodeNavigator
		for _, item := range v {
			if inner, ok := item.(*xml.InternalNode); ok {
				navs = append(navs, xpath.NewNavigator(inner))
			} else if ptr, ok := item.(unsafe.Pointer); ok {
				navs = append(navs, xpath.NewNavigator((*xml.InternalNode)(ptr)))
			}
		}
		if len(navs) > 0 {
			return navs
		}
	}
	return result
}

// normalizeXPathArg converts antchfx-level evaluation results (NodeNavigator,
// []NodeNavigator, string, float64, bool) into gokogiri types that XSLT
// functions expect.
func (context *ExecutionContext) normalizeXPathArg(arg interface{}) interface{} {
	// Check if it's a slice of NodeNavigators from function resolver
	type nodeAccessor interface {
		Node() xpath.NodeAdapter
	}
	switch v := arg.(type) {
	case []antchfx.NodeNavigator:
		var result []interface{}
		for _, nav := range v {
			if na, ok := nav.(nodeAccessor); ok {
				adapter := na.Node()
				if inner, ok := adapter.(*xml.InternalNode); ok {
					result = append(result, inner)
					continue
				}
				// Attribute results arrive as throw-away *xpath.AttrNode
				// copies. Materialise them as DOM attribute nodes carrying
				// their owning element instead of dropping them: EXSLT set
				// functions compare attributes by (owner, name) and iterate
				// the result, both of which need that owner.
				if attr, ok := adapter.(*xpath.AttrNode); ok {
					result = append(result, internalAttrNode(attr, navigatorOwner(nav)))
				}
				continue
			}
			// Synthetic navigators (the engine's scalarNavigator) are not DOM
			// nodes at all: they wrap a scalar value, e.g. a variable holding
			// a string. Hand the function that value the way string() would,
			// instead of dropping it -- dropping it made format-number($jobId,
			// '000000000') receive an empty node-set and print NaN.
			if nav != nil {
				result = append(result, nav.Value())
			}
		}
		return result
	}
	// Single NodeNavigator
	if na, ok := arg.(nodeAccessor); ok {
		adapter := na.Node()
		if inner, ok := adapter.(*xml.InternalNode); ok {
			return inner
		}
		if attr, ok := adapter.(*xpath.AttrNode); ok {
			if nav, isNav := arg.(antchfx.NodeNavigator); isNav {
				return internalAttrNode(attr, navigatorOwner(nav))
			}
			return internalAttrNode(attr, nil)
		}
		return adapter
	}
	if nav, ok := arg.(antchfx.NodeNavigator); ok && nav != nil {
		// Scalar in navigator clothing: use its string value.
		return nav.Value()
	}
	return arg
}

// xpathVarResolver adapts ExecutionContext to antchfx.VariableResolver.
type xpathVarResolver struct{ ctx *ExecutionContext }

func (r *xpathVarResolver) ResolveVariable(prefix, name string) (interface{}, error) {
	return r.ctx.ResolveXPathVariable(prefix, name)
}

// xpathFuncResolver adapts ExecutionContext to antchfx.FunctionResolver.
type xpathFuncResolver struct{ ctx *ExecutionContext }

func (r *xpathFuncResolver) ResolveFunction(prefix, name string, args []interface{}) (interface{}, error) {
	return r.ctx.ResolveXPathFunction(prefix, name, args)
}

// xpathResolvers returns VariableResolver and FunctionResolver adapters.
func (context *ExecutionContext) xpathResolvers() (antchfx.VariableResolver, antchfx.FunctionResolver) {
	return &xpathVarResolver{ctx: context}, &xpathFuncResolver{ctx: context}
}

// contextualizeXPath substitutes the XSLT context position and size into an
// XPath expression before it is handed to the XPath engine.
//
// XPath 1.0 defines position() as "the context position from the expression
// evaluation context" and XSLT defines that context position for an expression
// in an instruction as the position of the current node in the node list the
// current template/instruction is processing (xsl:for-each and
// xsl:apply-templates establish that list; xsl:call-template inherits it).
// The bundled XPath engine has no such context: its position() walks the
// previous nodes of the DOM tree and returns 1 + the number of them, which is
// only accidentally right when the node list is a run of adjacent siblings,
// and its last() applies the same idea forwards. For the REMITT stylesheets
// (statement.xsl renders its procedure rows with `$line + $offset` where
// $line is a position() from the enclosing for-each) that produced row 42
// where libxslt produces row 19.
//
// ratago therefore supplies the value itself, from the position/size it
// already tracks for the current node list (SetContextPosition in
// xsl:for-each, xsl:apply-templates and the match-pattern evaluator).
// Only top-level occurrences are substituted: inside a predicate the engine
// builds its own node list and its own position() is correct.
func (context *ExecutionContext) contextualizeXPath(expr string) string {
	if context.XPathContext == nil {
		return expr
	}
	pos, size := context.XPathContext.GetContextPosition()
	if pos < 1 {
		pos = 1
	}
	if size < 1 {
		size = 1
	}
	return rewriteContextPosition(expr, pos, size)
}

// rewriteContextPosition replaces top-level position()/last() calls in an
// XPath expression by the supplied literals. Calls inside a predicate
// (square brackets) or inside a string literal are left untouched. The
// expression is returned unchanged when there is nothing to replace.
func rewriteContextPosition(expr string, pos, size int) string {
	if !strings.Contains(expr, "position") && !strings.Contains(expr, "last") {
		return expr
	}
	var out strings.Builder
	depth := 0
	for i := 0; i < len(expr); {
		c := expr[i]
		switch c {
		case '\'', '"':
			// Copy the string literal verbatim: a function name inside a
			// literal is data, not a call.
			j := i + 1
			for j < len(expr) && expr[j] != c {
				j++
			}
			if j < len(expr) {
				j++
			}
			out.WriteString(expr[i:j])
			i = j
			continue
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		}
		if depth == 0 && isFunctionNameStart(expr, i) {
			if n, ok := matchZeroArgCall(expr[i:], "position"); ok {
				out.WriteString(strconv.Itoa(pos))
				i += n
				continue
			}
			if n, ok := matchZeroArgCall(expr[i:], "last"); ok {
				out.WriteString(strconv.Itoa(size))
				i += n
				continue
			}
		}
		out.WriteByte(c)
		i++
	}
	return out.String()
}

// isFunctionNameStart reports whether a function name may start at offset i,
// i.e. the preceding character cannot be part of a QName or a number
// ("my-position(" and "x:position(" are different functions).
func isFunctionNameStart(expr string, i int) bool {
	if i == 0 {
		return true
	}
	switch c := expr[i-1]; {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return false
	case c == '_' || c == '-' || c == '.' || c == ':':
		return false
	}
	return true
}

// matchZeroArgCall reports whether s starts with name followed by an empty
// argument list, optionally with whitespace around the parentheses, and
// returns the number of bytes that call spans.
func matchZeroArgCall(s, name string) (int, bool) {
	if !strings.HasPrefix(s, name) {
		return 0, false
	}
	i := len(name)
	for i < len(s) && (s[i] == ' ' || s[i] == '	' || s[i] == '\n' || s[i] == '\r') {
		i++
	}
	if i >= len(s) || s[i] != '(' {
		return 0, false
	}
	i++
	for i < len(s) && (s[i] == ' ' || s[i] == '	' || s[i] == '\n' || s[i] == '\r') {
		i++
	}
	if i >= len(s) || s[i] != ')' {
		return 0, false
	}
	return i + 1, true
}

func (context *ExecutionContext) DeclareLocalVariable(name, ns string, v *Variable) error {
	if context.Stack.Len() == 0 {
		return errors.New("Attempting to declare a local variable without a stack frame")
	}
	e := context.Stack.Front()
	scope := e.Value.(map[string]*Variable)
	scope[name] = v
	//fmt.Println("DECLARE", name, v)
	return nil
}

func (context *ExecutionContext) LookupLocalVariable(name, ns string) (ret *Variable) {
	for e := context.Stack.Front(); e != nil; e = e.Next() {
		scope := e.Value.(map[string]*Variable)
		v, ok := scope[name]
		if ok {
			//fmt.Println("FOUND", name, v)
			return v
		}
	}
	return
}

// create a local scope for variable resolution
func (context *ExecutionContext) PushStack() {
	scope := make(map[string]*Variable)
	context.Stack.PushFront(scope)
}

// leave the variable scope
func (context *ExecutionContext) PopStack() {
	if context.Stack.Len() == 0 {
		return
	}
	context.Stack.Remove(context.Stack.Front())
}

func (context *ExecutionContext) IsFunctionRegistered(name, ns string) bool {
	qname := fmt.Sprintf("{%s}%s", ns, name)
	_, ok := context.Style.Functions[qname]
	return ok
}

func (context *ExecutionContext) ResolveFunction(name, ns string) xpath.XPathFunction {
	qname := fmt.Sprintf("{%s}%s", ns, name)
	f, ok := context.Style.Functions[qname]
	if ok {
		return f
	}
	return nil
}

// Determine the default namespace currently defined in scope
func (context *ExecutionContext) DefaultNamespace(node xml.Node) string {
	//get the list of in-scope namespaces
	// any with a null prefix? return that
	decl := node.DeclaredNamespaces()
	for _, d := range decl {
		if d.Prefix == "" {
			return d.Uri
		}
	}
	return ""
}

// Propagate namespaces to the root of the output document
func (context *ExecutionContext) DeclareStylesheetNamespacesIfRoot(node xml.Node) {
	if context.OutputNode.NodeType() != xml.XML_DOCUMENT_NODE {
		return
	}
	//add all namespace declarations to r
	for uri, prefix := range context.Style.NamespaceMapping {
		if uri != XSLT_NAMESPACE && uri != XML_NAMESPACE {
			//these don't actually change if there is no alias
			_, uri = ResolveAlias(context.Style, prefix, uri)
			if !context.Style.IsExcluded(prefix) {
				node.DeclareNamespace(prefix, uri)
			}
		}
	}
}

func (context *ExecutionContext) FetchInputDocument(loc string, relativeToSource bool) (doc *xml.XmlDocument) {
	//create the map if needed
	if context.InputDocuments == nil {
		context.InputDocuments = make(map[string]*xml.XmlDocument)
	}

	// rely on caller to tell us how to resolve relative paths
	base := ""
	if relativeToSource {
		base, _ = filepath.Abs(filepath.Dir(context.Source.Uri()))
	} else {
		// Resolve against the owning stylesheet of the current template,
		// falling back to the top-level stylesheet
		if context.CurrentTemplate != nil && context.CurrentTemplate.OwningStyle != nil {
			base, _ = filepath.Abs(filepath.Dir(context.CurrentTemplate.OwningStyle.stylesheetUri))
		}
		if base == "" {
			base, _ = filepath.Abs(filepath.Dir(context.Style.Doc.Uri()))
		}
	}
	resolvedLoc := filepath.Join(base, loc)

	//if abspath in map return existing document
	doc, ok := context.InputDocuments[resolvedLoc]
	if ok {
		return
	}

	//else load the document and add to map
	doc, e := xml.ReadFile(resolvedLoc, xml.StrictParseOption)
	if e != nil {
		fmt.Println(e)
		return
	}
	context.InputDocuments[resolvedLoc] = doc
	return
}
