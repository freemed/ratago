package xslt

import (
	"fmt"
	"strings"

	"github.com/freemed/gokogiri/xml"
	"github.com/freemed/gokogiri/xpath"
)

const funcNamespace = "http://exslt.org/functions"

// UserFunction represents a func:function definition from EXSLT Functions.
type UserFunction struct {
	Name string // local name
	NS   string // namespace URI
	Args []string
	Body xml.Node // the func:function element node
}

// Apply executes the user-defined function when called from an XPath expression.
// This implements the xpath.XPathFunction signature.
func (uf *UserFunction) Apply(context xpath.VariableScope, args []interface{}) interface{} {
	c := context.(*ExecutionContext)

	// Create a temporary output document for capturing result
	output := xml.CreateEmptyDocument(c.Output.InputEncoding(), c.Output.OutputEncoding())
	oldOutput := c.Output
	oldOutputNode := c.OutputNode
	c.Output = output
	c.OutputNode = output.Node

	// Push local variable scope for parameters
	c.PushStack()
	defer func() {
		c.PopStack()
		c.Output = oldOutput
		c.OutputNode = oldOutputNode
	}()

	// Bind arguments to parameter names
	for i, name := range uf.Args {
		if i < len(args) {
			v := &Variable{Name: name, Value: args[i]}
			c.DeclareLocalVariable(name, "", v)
		}
	}

	// Execute the function body. Walk children looking for func:result.
	var resultValue interface{}
	for child := uf.Body.FirstChild(); child != nil; child = child.NextSibling() {
		if IsBlank(child) {
			continue
		}
		if child.NodeType() == xml.XML_COMMENT_NODE {
			continue
		}

		// Check for func:result
		if child.NodeType() == xml.XML_ELEMENT_NODE &&
			child.Namespace() == funcNamespace &&
			child.Name() == "result" {

			// Check for illegal siblings after func:result
			next := child.NextSibling()
			for next != nil {
				if IsBlank(next) {
					next = next.NextSibling()
					continue
				}
				if next.NodeType() == xml.XML_COMMENT_NODE {
					next = next.NextSibling()
					continue
				}
				// xsl:fallback is allowed
				if IsXsltName(next, "fallback") {
					next = next.NextSibling()
					continue
				}
				// Error: non-fallback sibling after func:result
				return nil
			}

			// Evaluate the func:result
			selectAttr := child.Attr("select")
			if selectAttr != "" {
				// select attribute takes precedence
				val, err := c.EvalXPath(c.Current, selectAttr)
				if err != nil {
					return nil
				}
				resultValue = val
			} else {
				// Evaluate children of func:result as a result tree fragment
				val := evaluateResultChildren(child, c)
				resultValue = val
			}
			break
		}

		// Check for xsl:param (skip - already handled)
		if IsXsltName(child, "param") {
			continue
		}

		// Check for xsl:variable
		if IsXsltName(child, "variable") {
			name := child.Attr("name")
			v := &Variable{Name: name}
			v.Compile(child)
			v.Apply(child, c)
			c.DeclareLocalVariable(name, "", v)
			continue
		}

		// Check for xsl:fallback
		if IsXsltName(child, "fallback") {
			continue
		}

		// Execute instruction children (xsl:value-of, xsl:if, etc.)
		if child.Namespace() == XSLT_NAMESPACE {
			instr := CompileSingleNode(child)
			instr.Compile(child)
			instr.Apply(c.Current, c)
			continue
		}

		// Literal result elements captured as result tree fragment
		applyNode(child, c)
	}

	// If no func:result was found, return the accumulated output as a node-set
	if resultValue == nil {
		// Return the output fragment as a node-set
		children := collectChildNodes(output)
		if len(children) > 0 {
			// Build a result tree fragment
			fauxroot := oldOutput.CreateElementNode("func-result")
			for _, ch := range children {
				fauxroot.AddChild(ch)
			}
			nodeset := xml.Nodeset{fauxroot}
			return nodeset.ToPointers()
		}
		return ""
	}

	return resultValue
}

// evaluateResultChildren evaluates the children of func:result and returns the value.
// If there's one text node child, returns its string value.
// If there are multiple children, returns a node-set.
func evaluateResultChildren(node xml.Node, c *ExecutionContext) interface{} {
	// Count non-blank children
	var children []xml.Node
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if IsBlank(child) {
			continue
		}
		children = append(children, child)
	}

	if len(children) == 1 && children[0].NodeType() == xml.XML_TEXT_NODE {
		return children[0].Content()
	}

	// Multiple children or non-text children: return as node-set
	if len(children) > 0 {
		fauxroot := c.Output.CreateElementNode("func-result-frag")
		for _, ch := range children {
			fauxroot.AddChild(ch.Duplicate(1))
		}
		nodeset := xml.Nodeset{fauxroot}
		return nodeset.ToPointers()
	}
	return ""
}

// collectChildNodes collects all child nodes of a document node.
func collectChildNodes(doc xml.Document) []xml.Node {
	var result []xml.Node
	root := doc.Root()
	if root == nil {
		return result
	}
	for cur := root.FirstChild(); cur != nil; cur = cur.NextSibling() {
		result = append(result, cur)
	}
	return result
}

// applyNode applies a single node in the context (helper for func:function body).
func applyNode(node xml.Node, c *ExecutionContext) {
	switch node.NodeType() {
	case xml.XML_ELEMENT_NODE:
		if node.Namespace() == XSLT_NAMESPACE {
			return // only instructions handled via CompileSingleNode above
		}
		elem := c.Output.CreateElementNode(node.Name())
		for _, ns := range node.DeclaredNamespaces() {
			elem.DeclareNamespace(ns.Prefix, ns.Uri)
		}
		// Copy attributes by iterating through known attribute names
		// gokogiri doesn't have an Attributes() iterator — attributes are
		// accessed by name via Attr().
		copyListedAttributes(node, elem)
		oldOutput := c.OutputNode
		c.OutputNode = elem
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			applyNode(child, c)
		}
		c.OutputNode = oldOutput
		oldOutput.AddChild(elem)
	case xml.XML_TEXT_NODE:
		txt := c.Output.CreateTextNode(node.Content())
		c.OutputNode.AddChild(txt)
	}
}

// copyListedAttributes copies standard attribute names from src to dst.
// gokogiri exposes attributes by name via Attr().
func copyListedAttributes(src, dst xml.Node) {
	// These are the common attribute names to check. A full implementation
	// would use a C-level attribute iterator, but gokogiri doesn't expose one.
	// For EXSLT func:function body elements, this covers the common cases.
	commonAttrs := []string{
		"select", "test", "name", "match", "use", "mode",
		"priority", "version", "href", "encoding", "method",
		"namespace", "exclude-result-prefixes", "extension-element-prefixes",
	}
	for _, attrName := range commonAttrs {
		val := src.Attr(attrName)
		if val != "" {
			dst.SetAttr(attrName, val)
		}
	}
}

// ParseUserFunction parses a func:function element and registers it in the stylesheet.
func (style *Stylesheet) ParseUserFunction(node xml.Node) error {
	nameAttr := node.Attr("name")
	if nameAttr == "" {
		return fmt.Errorf("func:function missing name attribute")
	}

	// Resolve QName to namespace + localname
	ns, local := resolveQNameForFunc(node, nameAttr, style)

	// Collect parameter names from xsl:param children
	var argNames []string
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if IsBlank(child) || child.NodeType() == xml.XML_COMMENT_NODE {
			continue
		}
		if IsXsltName(child, "param") {
			argNames = append(argNames, child.Attr("name"))
		}
	}

	uf := &UserFunction{
		Name: local,
		NS:   ns,
		Args: argNames,
		Body: node,
	}

	qname := fmt.Sprintf("{%s}%s", ns, local)
	style.Functions[qname] = uf.Apply
	return nil
}

// resolveQNameForFunc resolves a QName from a func:function element.
func resolveQNameForFunc(node xml.Node, qname string, style *Stylesheet) (ns, local string) {
	if !strings.Contains(qname, ":") {
		// No prefix - use default namespace from in-scope namespaces
		for _, decl := range node.DeclaredNamespaces() {
			if decl.Prefix == "" {
				return decl.Uri, qname
			}
		}
		return "", qname
	}
	parts := strings.SplitN(qname, ":", 2)
	prefix := parts[0]
	// Look up prefix in in-scope namespaces
	for _, decl := range node.DeclaredNamespaces() {
		if decl.Prefix == prefix {
			return decl.Uri, parts[1]
		}
	}
	// Fall back to stylesheet namespace mappings
	if uri, ok := style.namespaceForPrefix(prefix); ok {
		return uri, parts[1]
	}
	return "", parts[1]
}
