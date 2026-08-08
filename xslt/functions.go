package xslt

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unsafe"

	"github.com/freemed/gokogiri/xml"
	"github.com/freemed/gokogiri/xpath"
)

func (style *Stylesheet) RegisterXsltFunctions() {
	// id and lang are built into libxml2, don't need to be registered
	style.Functions["{}document"] = XsltDocumentFn
	style.Functions["{}generate-id"] = XsltGenerateId
	style.Functions["{}key"] = XsltKey
	style.Functions["{}system-property"] = XsltSystemProperty
	style.Functions["{}unparsed-entity-uri"] = XsltUnparsedEntityUri
	style.Functions["{}current"] = XsltCurrent
	style.Functions["{}element-available"] = XsltElementAvailable
	style.Functions["{}function-available"] = XsltFunctionAvailable
	style.Functions["{}format-number"] = XsltFormatNumber

	style.Functions["{http://xmlsoft.org/XSLT/namespace}node-set"] = EXSLTnodeset
	style.Functions["{http://exslt.org/common}node-set"] = EXSLTnodeset
	style.Functions["{http://exslt.org/math}constant"] = EXSLTmathconstant
	style.Functions["{http://exslt.org/math}sin"] = EXSLTmathsin
	style.Functions["{http://exslt.org/math}cos"] = EXSLTmathcos
	style.Functions["{http://exslt.org/math}abs"] = EXSLTmathabs
}

type Key struct {
	nodes map[string]xml.Nodeset
	use   string
	match string
}

// Implementation of key() from XSLT spec
func XsltKey(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 2 {
		return nil
	}
	// always convert to string
	name := argValToString(args[0])

	// convert to string
	val := argValToString(args[1])
	//get the execution context
	c := context.(*ExecutionContext)
	//look up the key
	keyList, ok := c.Style.Keys[name]
	if !ok {
		return nil
	}
	// Union results from all keys with the same name
	var result xml.Nodeset
	for _, k := range keyList {
		nodes, _ := k.nodes[val]
		result = append(result, nodes...)
	}
	//return the nodeset
	return result.ToPointers()
}

// Implementation of system-property() from XSLT spec
func XsltSystemProperty(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	switch args[0].(string) {
	case "xsl:version":
		return 1.0
	case "xsl:vendor":
		return "John C Barstow"
	case "xsl:vendor-url":
		return "http://github.com/freemed/ratago"
	default:
		fmt.Println("EXEC system-property", args[0])
	}
	return nil
}

//Implementation of document() from XSLT spec
func XsltDocumentFn(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	c := context.(*ExecutionContext)

	switch doc := args[0].(type) {
	case string:
		if doc == "" {
			nodeset := xml.Nodeset{c.Style.Doc}
			return nodeset.ToPointers()
		}
		input := c.FetchInputDocument(doc, false)
		if input != nil {
			nodeset := xml.Nodeset{input}
			return nodeset.ToPointers()
		}
		return nil
	case []unsafe.Pointer:
		n := xml.NewNode(doc[0], nil)
		location := n.Content()
		input := c.FetchInputDocument(location, true)
		if input != nil {
			nodeset := xml.Nodeset{input}
			return nodeset.ToPointers()
		}
		fmt.Println("DOCUMENT", location)
	}
	return nil
}

// Implementation of generate-id() from XSLT spec
func XsltGenerateId(context xpath.VariableScope, args []interface{}) interface{} {
	// should be 0 or 1 argument
	if len(args) > 1 {
		return nil
	}

	c := context.(*ExecutionContext)
	// When called with no argument, generate-id for the context node
	if len(args) < 1 {
		if c.Current != nil {
			out := fmt.Sprintf("N%v", uintptr(c.Current.NodePtr()))
			return out
		}
		return "N"
	}

	switch v := args[0].(type) {
	case []unsafe.Pointer:
		if len(v) == 0 {
			return nil
		}
		out := fmt.Sprintf("N%v", uintptr(v[0]))
		return out
	default:
		return nil
	}
}

// Implementation of unparsed-entity-uri() from XSLT spec
func XsltUnparsedEntityUri(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	c := context.(*ExecutionContext)
	name := argValToString(args[0])
	val := c.Source.UnparsedEntityURI(name)
	return val
}

// Implementation of current() from XSLT spec
func XsltCurrent(context xpath.VariableScope, args []interface{}) interface{} {
	c := context.(*ExecutionContext)
	n := xml.Nodeset{c.Current}
	return n.ToPointers()
}

// Implementation of function-available() from XSLT spec
func XsltFunctionAvailable(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	c := context.(*ExecutionContext)
	qname := args[0].(string)
	// Resolve namespace from QName
	ns, local := "", qname
	if strings.Contains(qname, ":") {
		parts := strings.SplitN(qname, ":", 2)
		ns = c.LookupNamespace(parts[0], c.Current)
		local = parts[1]
	}
	return c.IsFunctionRegistered(ns, local)
}

// Implementation of element-available() from XSLT spec
func XsltElementAvailable(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	c := context.(*ExecutionContext)
	qname := args[0].(string)
	// Resolve namespace from QName
	ns, local := "", qname
	if strings.Contains(qname, ":") {
		parts := strings.SplitN(qname, ":", 2)
		ns = c.LookupNamespace(parts[0], c.Current)
		local = parts[1]
	}
	// XSLT namespace elements are always available
	if ns == XSLT_NAMESPACE || ns == "" {
		switch local {
		case "apply-imports", "apply-templates", "attribute", "attribute-set",
			"call-template", "choose", "comment", "copy", "copy-of",
			"decimal-format", "element", "fallback", "for-each",
			"if", "import", "include", "key", "message",
			"namespace-alias", "number", "otherwise", "output",
			"param", "preserve-space", "processing-instruction",
			"sort", "strip-space", "stylesheet", "template",
			"text", "transform", "value-of", "variable", "when",
			"with-param":
			return true
		}
	}
	return false
}

// util function because we can't assume we're actually getting a string
func argValToString(val interface{}) (out string) {
	if val == nil {
		return
	}
	switch v := val.(type) {
	case string:
		return v
	case []unsafe.Pointer:
		if len(v) == 0 {
			return
		}
		n := xml.NewNode(v[0], nil)
		out = n.Content()
	default:
		out = fmt.Sprintf("%v", v)
	}
	return
}

func EXSLTnodeset(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	c := context.(*ExecutionContext)
	nodes := args[0]
	switch v := nodes.(type) {
	case []unsafe.Pointer:
		if len(v) == 0 {
			return nil
		}
		fauxroot := c.Output.CreateElementNode("VARIABLE")
		for _, node := range v {
			n := xml.NewNode(node, nil)
			fauxroot.AddChild(n)
		}
		out := xml.Nodeset{fauxroot}
		return out.ToPointers()
	default:
		out := fmt.Sprintf("%v", v)
		fmt.Println("invalid argument to exslt:nodeset", out)
	}

	return nodes
}

func EXSLTmathconstant(context xpath.VariableScope, args []interface{}) interface{} {

	if len(args) != 2 {
		return 0
	}

	name := args[0].(string)
	precision := int(args[1].(float64))

	switch name {
	case "PI":
		return fmt.Sprintf("%.*f", precision, 3.1415926535897932384626433832795028841971693993751)
	case "E":
		return fmt.Sprintf("%.*f", precision, 2.71828182845904523536028747135266249775724709369996)
	case "SQRRT2":
		return fmt.Sprintf("%.*f", precision, 1.41421356237309504880168872420969807856967187537694)
	case "LN2":
		return fmt.Sprintf("%.*f", precision, 0.69314718055994530941723212145817656807550013436025)
	case "LN10":
		return fmt.Sprintf("%.*f", precision, 2.30258509299404568402)
	case "LOG2E":
		return fmt.Sprintf("%.*f", precision, 1.4426950408889634074)
	case "SQRT1_2":
		return fmt.Sprintf("%.*f", precision, 0.70710678118654752440)
	default:
		out := fmt.Sprintf("%v", name)
		fmt.Println("unsupported constant in math:constant", out)
	}

	return 0
}

func EXSLTmathsin(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 1 {
		return nil
	}

	return math.Sin(args[0].(float64))
}

func EXSLTmathcos(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 1 {
		return nil
	}

	return math.Cos(args[0].(float64))
}

func EXSLTmathabs(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 1 {
		return nil
	}

	return math.Abs(args[0].(float64))
}

func XsltFormatNumber(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 2 {
		return nil
	}

	number := args[0].(float64)
	format := args[1].(string)

	if len(format) <= 0 {
		fmt.Println("XsltFormatNumber: Invalid format (0-length)")
	}

	// Determine which decimal-format to use
	c := context.(*ExecutionContext)
	var df *DecimalFormat
	if len(args) >= 3 {
		dfName := argValToString(args[2])
		if dfName != "" {
			df = c.Style.DecimalFormats[dfName]
		}
	}
	if df == nil {
		df = c.Style.DecimalFormats[""] // default unnamed format
	}

	// Handle special values
	if math.IsNaN(number) {
		if df != nil && df.NaN != "" {
			return df.NaN
		}
		return "NaN"
	}
	if math.IsInf(number, 1) {
		if df != nil && df.Infinity != "" {
			return df.Infinity
		}
		return "Infinity"
	}
	if math.IsInf(number, -1) {
		if df != nil && df.Infinity != "" {
			minusSign := "-"
			if df != nil && df.MinusSign != "" {
				minusSign = df.MinusSign
			}
			return minusSign + df.Infinity
		}
		return "-Infinity"
	}

	// Determine separators
	decSep := "."
	groupSep := ""
	if df != nil {
		if df.DecimalSeparator != "" {
			decSep = df.DecimalSeparator
		}
		if df.GroupingSeparator != "" {
			groupSep = df.GroupingSeparator
		}
	}

	// Handle negative numbers
	negative := number < 0
	if negative {
		number = -number
	}

	re := regexp.MustCompile("(?P<int>(?:#|0)*)(?P<dot>.)(?P<dec>(?:#|0)*)")
	names := re.SubexpNames()
	matches := re.FindAllStringSubmatch(format, -1)
	if matches == nil || len(matches) == 0 {
		// No decimal part in format; format as integer
		intPart := reInteger.FindAllStringSubmatch(format, -1)
		if intPart != nil {
			parts := map[string]string{}
			for i, n := range reInteger.SubexpNames() {
				parts[n] = intPart[0][i]
			}
			return formatIntegerPart(number, parts, groupSep, negative, df)
		}
		return fmt.Sprintf("%v", number)
	}

	parts := map[string]string{}
	for i, n := range matches[0] {
		parts[names[i]] = n
	}

	var buffer bytes.Buffer

	// Format integer part
	intStr := formatIntegerPart(number, parts, groupSep, false, df)
	buffer.WriteString(intStr)

	// Format decimal part
	if parts["dot"] != "" {
		buffer.WriteString(decSep)
		frac := number - float64(int64(number))
		if frac < 0 {
			frac = -frac
		}
		decFmt := parts["dec"]
		if decFmt != "" {
			decStr := strconv.FormatFloat(frac, 'f', len(decFmt), 64)
			// Strip leading "0."
			if len(decStr) > 2 && decStr[0] == '0' && decStr[1] == '.' {
				decStr = decStr[2:]
			}
			buffer.WriteString(decStr)
		}
	}

	result := buffer.String()

	// Prepend minus sign for negative numbers
	if negative {
		minusSign := "-"
		if df != nil && df.MinusSign != "" {
			minusSign = df.MinusSign
		}
		result = minusSign + result
	}

	return result
}

var reInteger = regexp.MustCompile("(?P<int>(?:#|0)*)")

func formatIntegerPart(number float64, parts map[string]string, groupSep string, negative bool, df *DecimalFormat) string {
	var buffer bytes.Buffer
	intPart := parts["int"]

	intstr := strconv.FormatInt(int64(number), 10)

	if intPart != "" {
		// If format is all zeros, pad with leading zeros
		if !strings.ContainsRune(intPart, '#') {
			if len(intstr) < len(intPart) {
				for i := 0; i < len(intPart)-len(intstr); i++ {
					buffer.WriteByte('0')
				}
			}
			buffer.WriteString(intstr)
		} else {
			buffer.WriteString(intstr)
		}

		// Apply grouping separator if specified
		if groupSep != "" {
			result := buffer.String()
			buffer.Reset()
			for i, ch := range result {
				if i > 0 && (len(result)-i)%3 == 0 {
					buffer.WriteString(groupSep)
				}
				buffer.WriteRune(ch)
			}
		}
	}

	return buffer.String()
}
