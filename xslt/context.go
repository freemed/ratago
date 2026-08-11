package xslt

import (
	"container/list"
	"errors"
	"fmt"
	"github.com/freemed/gokogiri/xml"
	"github.com/freemed/gokogiri/xpath"
	antchfx "github.com/freemed/xpath"
	"path/filepath"
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
		if xpathExpr != nil {
			defer xpathExpr.Free()
			result, err = context.EvalXPath(xmlNode, xpathExpr)
		} else {
			err = errors.New("cannot compile xpath: " + data)
		}
	case []byte:
		result, err = context.EvalXPath(xmlNode, string(data))
	case *xpath.Expression:
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

// ResolveXPathVariable implements antchfx.VariableResolver for the XPath engine.
// It resolves $variable references in XPath expressions by looking up the
// variable in local scope, then global scope. Returns the value in a form
// compatible with antchfx/xpath: string, float64, bool, or NodeNavigator.
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
		if len(val) > 0 {
			var navs []antchfx.NodeNavigator
			for _, n := range val {
				navs = append(navs, xpath.NewNavigator(n.NodePtr().(xpath.NodeAdapter)))
			}
			if len(navs) == 1 {
				return navs[0], nil
			}
			return navs, nil
		}
		return "", nil
	case []xml.Node:
		if len(val) > 0 {
			var navs []antchfx.NodeNavigator
			for _, n := range val {
				navs = append(navs, xpath.NewNavigator(n.NodePtr().(xpath.NodeAdapter)))
			}
			if len(navs) == 1 {
				return navs[0], nil
			}
			return navs, nil
		}
		return "", nil
	case []unsafe.Pointer:
		if len(val) > 0 {
			var navs []antchfx.NodeNavigator
			for _, p := range val {
				inner := (*xml.InternalNode)(p)
				navs = append(navs, xpath.NewNavigator(inner))
			}
			if len(navs) == 1 {
				return navs[0], nil
			}
			return navs, nil
		}
		return "", nil
	case []interface{}:
		if len(val) > 0 {
			var navs []antchfx.NodeNavigator
			for _, item := range val {
				if inner, ok := item.(*xml.InternalNode); ok {
					navs = append(navs, xpath.NewNavigator(inner))
				} else if ptr, ok := item.(unsafe.Pointer); ok {
					navs = append(navs, xpath.NewNavigator((*xml.InternalNode)(ptr)))
				}
			}
			if len(navs) == 1 {
				return navs[0], nil
			}
			if len(navs) > 0 {
				return navs, nil
			}
		}
		return "", nil
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
				}
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
		return adapter
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
