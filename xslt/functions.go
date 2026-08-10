package xslt

import (
	"bytes"
	"fmt"
	"math"
	"math/rand/v2"
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
	style.Functions["{http://exslt.org/common}object-type"] = EXSLTobjectType
	style.Functions["{http://exslt.org/math}constant"] = EXSLTmathconstant
	style.Functions["{http://exslt.org/math}sin"] = EXSLTmathsin
	style.Functions["{http://exslt.org/math}cos"] = EXSLTmathcos
	style.Functions["{http://exslt.org/math}abs"] = EXSLTmathabs
	style.Functions["{http://exslt.org/math}min"] = EXSLTmathmin
	style.Functions["{http://exslt.org/math}max"] = EXSLTmathmax
	style.Functions["{http://exslt.org/math}sqrt"] = EXSLTmathsqrt
	style.Functions["{http://exslt.org/math}power"] = EXSLTmathpower
	style.Functions["{http://exslt.org/math}tan"] = EXSLTmathtan
	style.Functions["{http://exslt.org/math}asin"] = EXSLTmathasin
	style.Functions["{http://exslt.org/math}acos"] = EXSLTmathacos
	style.Functions["{http://exslt.org/math}atan"] = EXSLTmathatan
	style.Functions["{http://exslt.org/math}atan2"] = EXSLTmathatan2
	style.Functions["{http://exslt.org/math}exp"] = EXSLTmathexp
	style.Functions["{http://exslt.org/math}log"] = EXSLTmathlog
	style.Functions["{http://exslt.org/math}random"] = EXSLTmathrandom
	style.Functions["{http://exslt.org/math}highest"] = EXSLTmathhighest
	style.Functions["{http://exslt.org/math}lowest"] = EXSLTmathlowest
	style.Functions["{http://exslt.org/sets}difference"] = EXSLTsetDifference
	style.Functions["{http://exslt.org/sets}intersection"] = EXSLTsetIntersection
	style.Functions["{http://exslt.org/sets}distinct"] = EXSLTsetDistinct
	style.Functions["{http://exslt.org/sets}has-same-node"] = EXSLTsetHasSameNode
	style.Functions["{http://exslt.org/sets}leading"] = EXSLTsetLeading
	style.Functions["{http://exslt.org/sets}trailing"] = EXSLTsetTrailing
	style.Functions["{http://exslt.org/strings}concat"] = EXSLTstrConcat
	style.Functions["{http://exslt.org/strings}split"] = EXSLTstrSplit
	style.Functions["{http://exslt.org/strings}tokenize"] = EXSLTstrSplit
	style.Functions["{http://exslt.org/strings}replace"] = EXSLTstrReplace
	style.Functions["{http://exslt.org/strings}padding"] = EXSLTstrPadding
	style.Functions["{http://exslt.org/strings}align"] = EXSLTstrAlign
	style.Functions["{http://exslt.org/dynamic}evaluate"] = EXSLTdynEvaluate
	style.Functions["{http://exslt.org/random}random-sequence"] = EXSLTrndRandomSequence
	style.Functions["{http://exslt.org/dates-and-times}date-time"] = EXSLTdateDateTime
	style.Functions["{http://exslt.org/dates-and-times}date"] = EXSLTdateDate
	style.Functions["{http://exslt.org/dates-and-times}time"] = EXSLTdateTime
	style.Functions["{http://exslt.org/dates-and-times}year"] = EXSLTdateYear
	style.Functions["{http://exslt.org/dates-and-times}month-in-year"] = EXSLTdateMonthInYear
	style.Functions["{http://exslt.org/dates-and-times}day-in-month"] = EXSLTdateDayInMonth
	style.Functions["{http://exslt.org/dates-and-times}day-in-year"] = EXSLTdateDayInYear
	style.Functions["{http://exslt.org/dates-and-times}hour-in-day"] = EXSLTdateHourInDay
	style.Functions["{http://exslt.org/dates-and-times}minute-in-hour"] = EXSLTdateMinuteInHour
	style.Functions["{http://exslt.org/dates-and-times}second-in-minute"] = EXSLTdateSecondInMinute
	style.Functions["{http://exslt.org/dates-and-times}week-in-year"] = EXSLTdateWeekInYear
	style.Functions["{http://exslt.org/dates-and-times}day-in-week"] = EXSLTdateDayInWeek
	style.Functions["{http://exslt.org/dates-and-times}add"] = EXSLTdateAdd
	style.Functions["{http://exslt.org/dates-and-times}add-duration"] = EXSLTdateAddDuration
	style.Functions["{http://exslt.org/dates-and-times}duration"] = EXSLTdateDuration
	style.Functions["{http://exslt.org/dates-and-times}sum"] = EXSLTdateSum
	style.Functions["{http://exslt.org/dates-and-times}seconds"] = EXSLTdateSeconds
	style.Functions["{http://exslt.org/dates-and-times}difference"] = EXSLTdateDifference
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
			nodeset := xml.Nodeset{c.Style.Doc.Node}
				return nodeset.ToPointers()
			}
			input := c.FetchInputDocument(doc, false)
			if input != nil {
				nodeset := xml.Nodeset{input.Node}
				return nodeset.ToPointers()
			}
			return nil
			case []interface{}:
			n := xml.NewNode(doc[0].(*xml.InternalNode), nil)
			location := n.Content()
			input := c.FetchInputDocument(location, true)
			if input != nil {
				nodeset := xml.Nodeset{input.Node}
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
			out := fmt.Sprintf("N%p", c.Current.NodePtr())
			return out
		}
		return "N"
	}

	switch v := args[0].(type) {
	case []interface{}:
		if len(v) == 0 {
			return nil
		}
		out := fmt.Sprintf("N%p", v[0])
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
	case []interface{}:
		if len(v) == 0 {
			return
		}
		n := xml.NewNode(v[0].(*xml.InternalNode), nil)
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
	case []interface{}:
		if len(v) == 0 {
			return nil
		}
		fauxroot := c.Output.CreateElementNode("VARIABLE")
		for _, node := range v {
			n := xml.NewNode(node.(*xml.InternalNode), nil)
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

// nodeSetToFloat64s extracts numeric values from a node-set.
// Returns NaN for nodes that can't be parsed as numbers.
func nodeSetToFloat64s(arg interface{}) []float64 {
	switch v := arg.(type) {
	case []interface{}:
		result := make([]float64, 0, len(v))
		for _, ptr := range v {
			n := xml.NewNode(ptr.(*xml.InternalNode), nil)
			f, err := strconv.ParseFloat(strings.TrimSpace(n.String()), 64)
			if err != nil {
				result = append(result, math.NaN())
			} else {
				result = append(result, f)
			}
		}
		return result
	case float64:
		return []float64{v}
	default:
		return nil
	}
}

func EXSLTmathmin(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return math.NaN()
	}
	values := nodeSetToFloat64s(args[0])
	if len(values) == 0 {
		return math.NaN()
	}
	min := values[0]
	for _, v := range values[1:] {
		if !math.IsNaN(v) && (math.IsNaN(min) || v < min) {
			min = v
		}
	}
	return min
}

func EXSLTmathmax(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return math.NaN()
	}
	values := nodeSetToFloat64s(args[0])
	if len(values) == 0 {
		return math.NaN()
	}
	max := values[0]
	for _, v := range values[1:] {
		if !math.IsNaN(v) && (math.IsNaN(max) || v > max) {
			max = v
		}
	}
	return max
}

func EXSLTmathsqrt(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 1 {
		return math.NaN()
	}
	return math.Sqrt(args[0].(float64))
}

func EXSLTmathpower(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 2 {
		return math.NaN()
	}
	return math.Pow(args[0].(float64), args[1].(float64))
}

func EXSLTmathtan(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 1 {
		return nil
	}
	return math.Tan(args[0].(float64))
}

func EXSLTmathasin(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 1 {
		return nil
	}
	val := args[0].(float64)
	if val < -1 {
		val = -1
	}
	if val > 1 {
		val = 1
	}
	return math.Asin(val)
}

func EXSLTmathacos(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 1 {
		return nil
	}
	val := args[0].(float64)
	if val < -1 {
		val = -1
	}
	if val > 1 {
		val = 1
	}
	return math.Acos(val)
}

func EXSLTmathatan(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 1 {
		return nil
	}
	return math.Atan(args[0].(float64))
}

func EXSLTmathatan2(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 2 {
		return nil
	}
	return math.Atan2(args[0].(float64), args[1].(float64))
}

func EXSLTmathexp(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 1 {
		return nil
	}
	return math.Exp(args[0].(float64))
}

func EXSLTmathlog(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 1 {
		return nil
	}
	val := args[0].(float64)
	if val <= 0 {
		return math.NaN()
	}
	return math.Log(val)
}

var mathRandomSource *rand.Rand

func EXSLTmathrandom(context xpath.VariableScope, args []interface{}) interface{} {
	switch len(args) {
	case 0:
		if mathRandomSource == nil {
			mathRandomSource = rand.New(rand.NewPCG(0, 0))
		}
		return mathRandomSource.Float64()
	case 1:
		switch v := args[0].(type) {
		case bool:
			if v {
				// Reseed with current time
				mathRandomSource = rand.New(rand.NewPCG(uint64(rand.Uint64()), uint64(rand.Uint64())))
				return mathRandomSource.Float64()
			}
			return mathRandomSource.Float64()
		case float64:
			seed := uint64(v)
			mathRandomSource = rand.New(rand.NewPCG(seed, seed+1))
			return mathRandomSource.Float64()
		default:
			return nil
		}
	default:
		return nil
	}
}

func EXSLTmathhighest(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	nodes, ok := args[0].([]interface{})
	if !ok || len(nodes) == 0 {
		return nil
	}
	values := nodeSetToFloat64s(args[0])
	if len(values) == 0 {
		return nil
	}
	// Find the max value
	max := values[0]
	for _, v := range values[1:] {
		if !math.IsNaN(v) && (math.IsNaN(max) || v > max) {
			max = v
		}
	}
	// Collect all nodes with that value
	var result []interface{}
	for i, v := range values {
		if v == max {
			result = append(result, nodes[i])
		}
	}
	return result
}

func EXSLTmathlowest(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	nodes, ok := args[0].([]interface{})
	if !ok || len(nodes) == 0 {
		return nil
	}
	values := nodeSetToFloat64s(args[0])
	if len(values) == 0 {
		return nil
	}
	// Find the min value
	min := values[0]
	for _, v := range values[1:] {
		if !math.IsNaN(v) && (math.IsNaN(min) || v < min) {
			min = v
		}
	}
	// Collect all nodes with that value
	var result []interface{}
	for i, v := range values {
		if v == min {
			result = append(result, nodes[i])
		}
	}
	return result
}

// ---------- EXSLT Common ----------

func EXSLTobjectType(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return "string"
	}
	switch args[0].(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case []interface{}:
		return "node-set"
	default:
		return "string"
	}
}

// ---------- EXSLT Sets ----------

// nodeSetFromPointers converts []interface{} to a flat slice of nodesets
// by extracting the root pointers.
func nodeSetFromPointers(arg interface{}) ([]interface{}, bool) {
	switch v := arg.(type) {
	case []interface{}:
		return v, true
	}
	return nil, false
}

// inNodeSet checks if a pointer exists in a node-set by identity.
func inNodeSet(set []interface{}, ptr interface{}) bool {
	for _, p := range set {
		if p == ptr {
			return true
		}
	}
	return false
}

func EXSLTsetDifference(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 2 {
		return nil
	}
	n1, ok1 := nodeSetFromPointers(args[0])
	n2, ok2 := nodeSetFromPointers(args[1])
	if !ok1 || !ok2 {
		return nil
	}
	var result []interface{}
	for _, p := range n1 {
		if !inNodeSet(n2, p) {
			result = append(result, p)
		}
	}
	return result
}

func EXSLTsetIntersection(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 2 {
		return nil
	}
	n1, ok1 := nodeSetFromPointers(args[0])
	n2, ok2 := nodeSetFromPointers(args[1])
	if !ok1 || !ok2 {
		return nil
	}
	var result []interface{}
	for _, p := range n1 {
		if inNodeSet(n2, p) {
			result = append(result, p)
		}
	}
	return result
}

func EXSLTsetDistinct(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 1 {
		return nil
	}
	nodes, ok := nodeSetFromPointers(args[0])
	if !ok {
		return nil
	}
	seen := make(map[string]bool)
	var result []interface{}
	for _, p := range nodes {
		n := xml.NewNode(p.(*xml.InternalNode), nil)
		val := strings.TrimSpace(n.String())
		if !seen[val] {
			seen[val] = true
			result = append(result, p)
		}
	}
	return result
}

func EXSLTsetHasSameNode(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 2 {
		return false
	}
	n1, ok1 := nodeSetFromPointers(args[0])
	n2, ok2 := nodeSetFromPointers(args[1])
	if !ok1 || !ok2 {
		return false
	}
	for _, p1 := range n1 {
		if inNodeSet(n2, p1) {
			return true
		}
	}
	return false
}

func EXSLTsetLeading(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 2 {
		return nil
	}
	n1, ok1 := nodeSetFromPointers(args[0])
	n2, ok2 := nodeSetFromPointers(args[1])
	if !ok1 || !ok2 || len(n2) == 0 {
		return nil
	}

	// Find the first node from n2 in document order.
	// In libxml2, node pointers are in document order for nodes in the same doc,
	// so the smallest pointer comes first.
	firstPtr := n2[0]
	for _, p := range n2[1:] {
		if uintptr(unsafe.Pointer(p.(*xml.InternalNode))) < uintptr(unsafe.Pointer(firstPtr.(*xml.InternalNode))) {
			firstPtr = p
		}
	}

	// Return nodes from n1 that come before firstPtr in document order.
	var result []interface{}
	for _, p := range n1 {
		if uintptr(unsafe.Pointer(p.(*xml.InternalNode))) < uintptr(unsafe.Pointer(firstPtr.(*xml.InternalNode))) {
			result = append(result, p)
		}
	}
	return result
}

func EXSLTsetTrailing(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 2 {
		return nil
	}
	n1, ok1 := nodeSetFromPointers(args[0])
	n2, ok2 := nodeSetFromPointers(args[1])
	if !ok1 || !ok2 || len(n2) == 0 {
		return nil
	}

	// Find the first node from n2 in document order.
	firstPtr := n2[0]
	for _, p := range n2[1:] {
		if uintptr(unsafe.Pointer(p.(*xml.InternalNode))) < uintptr(unsafe.Pointer(firstPtr.(*xml.InternalNode))) {
			firstPtr = p
		}
	}

	// Return nodes from n1 that come after firstPtr in document order.
	var result []interface{}
	for _, p := range n1 {
		if uintptr(unsafe.Pointer(p.(*xml.InternalNode))) > uintptr(unsafe.Pointer(firstPtr.(*xml.InternalNode))) {
			result = append(result, p)
		}
	}
	return result
}

// ---------- EXSLT Strings ----------

const strNamespace = "http://exslt.org/strings"

func EXSLTstrConcat(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return ""
	}
	nodes, ok := nodeSetFromPointers(args[0])
	if !ok {
		return ""
	}
	var sep string
	if len(args) >= 2 {
		sep = argValToString(args[1])
	}
	var parts []string
	for _, p := range nodes {
		n := xml.NewNode(p.(*xml.InternalNode), nil)
		parts = append(parts, n.String())
	}
	return strings.Join(parts, sep)
}

func EXSLTstrSplit(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	c := context.(*ExecutionContext)
	val := argValToString(args[0])

	var tokens []string
	if len(args) >= 2 {
		pattern := argValToString(args[1])
		if pattern != "" {
			tokens = strings.Split(val, pattern)
		} else {
			tokens = strings.Fields(val)
		}
	} else {
		tokens = strings.Fields(val)
	}

	// Build a result tree fragment with <token> elements.
	fauxroot := c.Output.CreateElementNode("token-wrapper")
	for _, tok := range tokens {
		tokenElem := c.Output.CreateElementNode("token")
		tokenElem.SetNamespace(strNamespace, "")
		tokenText := c.Output.CreateTextNode(tok)
		tokenElem.AddChild(tokenText)
		fauxroot.AddChild(tokenElem)
	}
	nodeset := xml.Nodeset{fauxroot}
	return nodeset.ToPointers()
}

func EXSLTstrReplace(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 3 {
		return ""
	}
	str := argValToString(args[0])
	search := argValToString(args[1])
	replace := argValToString(args[2])
	return strings.ReplaceAll(str, search, replace)
}

func EXSLTstrPadding(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return ""
	}
	length := int(args[0].(float64))
	if length <= 0 {
		return ""
	}
	char := " "
	if len(args) >= 2 {
		s := argValToString(args[1])
		if len(s) > 0 {
			char = string(s[0])
		}
	}
	if len(char) == 0 {
		char = " "
	}
	return strings.Repeat(char, length)
}

func EXSLTstrAlign(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 2 {
		return ""
	}
	target := argValToString(args[0])
	align := argValToString(args[1])

	// Determine padding character
	padChar := " "
	if len(args) >= 3 {
		s := argValToString(args[2])
		if len(s) > 0 {
			padChar = string(s[0])
		}
	}
	if padChar == "" {
		padChar = " "
	}

	// The second argument is a template string whose length determines the field width.
	width := len(align)

	if len(target) >= width {
		return target[:width]
	}

	switch {
	case strings.HasPrefix(align, "left"):
		return target + strings.Repeat(padChar, width-len(target))
	case strings.HasPrefix(align, "right"):
		return strings.Repeat(padChar, width-len(target)) + target
	case strings.HasPrefix(align, "center"):
		leftPad := (width - len(target)) / 2
		rightPad := width - len(target) - leftPad
		return strings.Repeat(padChar, leftPad) + target + strings.Repeat(padChar, rightPad)
	default:
		// align is a literal pad string: right-justify within it
		return strings.Repeat(padChar, width-len(target)) + target
	}
}

// ---------- EXSLT Dynamic ----------

func EXSLTdynEvaluate(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	c := context.(*ExecutionContext)
	xpathStr := argValToString(args[0])

	evalNode := c.Current
	if len(args) >= 2 {
		switch v := args[1].(type) {
		case []interface{}:
			if len(v) > 0 {
				evalNode = xml.NewNode(v[0].(*xml.InternalNode), nil)
			}
		}
	}

	result, err := c.EvalXPath(evalNode, xpathStr)
	if err != nil {
		return nil
	}
	return result
}

// ---------- EXSLT Random ----------

func EXSLTrndRandomSequence(context xpath.VariableScope, args []interface{}) interface{} {
	c := context.(*ExecutionContext)
	var count int
	var seed int64

	switch len(args) {
	case 0:
		count = 1
	case 1:
		count = int(args[0].(float64))
		if count < 1 {
			count = 1
		}
	case 2:
		seed = int64(args[0].(float64))
		count = int(args[1].(float64))
		if count < 1 {
			count = 1
		}
	default:
		return nil
	}

	var rng *rand.Rand
	if len(args) >= 2 {
		rng = rand.New(rand.NewPCG(uint64(seed), uint64(seed<<32)))
	} else {
		rng = rand.New(rand.NewPCG(uint64(rand.Uint64()), uint64(rand.Uint64())))
	}

	if count == 1 {
		return rng.Float64()
	}

	fauxroot := c.Output.CreateElementNode("random-sequence-wrapper")
	for i := 0; i < count; i++ {
		elem := c.Output.CreateElementNode("random-sequence")
		elem.SetNamespace("http://exslt.org/random", "")
		txt := c.Output.CreateTextNode(fmt.Sprintf("%f", rng.Float64()))
		elem.AddChild(txt)
		fauxroot.AddChild(elem)
	}
	nodeset := xml.Nodeset{fauxroot}
	return nodeset.ToPointers()
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
