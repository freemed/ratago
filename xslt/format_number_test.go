package xslt

// Regression tests for format-number() and the XPath 1.0 number conversion it
// runs on its first argument.
//
// Three defects these tests guard against, each of which changed real output of
// the shipped REMITT stylesheets (resources/xsl/*.xsl) away from what libxslt
// produces:
//
//  1. Pattern splitting. format-number()'s pattern is the JDK 1.1
//     DecimalFormat pattern, so only the decimal-separator CHARACTER starts the
//     fractional part. The old implementation split the pattern with the regex
//     ((?:#|0)*)(.)((?:#|0)*), i.e. it treated whatever character followed the
//     leading run of '#'/'0' as a decimal separator. With the all-zero pattern
//     "000000000" that matched "00000000" + "." + "" and
//     format-number($jobId, '000000000') printed "00000000." instead of
//     "000000001" -- corrupting the 837P interchange control number.
//     The same splitter also had to learn libxslt's "0" rules: a '#'-only
//     integer part renders nothing for a value below 1 ('.00' for
//     format-number(0, '#.00')), the mandatory '0' digits left-pad
//     ("000000001"), and rounding happens before the integer/fraction split so
//     that it carries (format-number(0.5, '#') is "1", format-number(2.999,
//     '0.00') is "3.00").
//
//  2. Number conversion of the first argument. XPath 1.0 number() rules apply:
//     an empty node-set is NaN (NOT 0) and so is a string that is not a valid
//     XPath number. Treating those as 0 made cms1500 box 24f and the statement
//     charge/paid/balance columns print "0.00" where xsltproc prints "NaN".
//
//  3. Scalar arguments to extension functions used to be dropped on the floor:
//     the engine wrapped a variable holding a string in a synthetic navigator,
//     and the argument conversion discarded navigators that are not DOM nodes.
//     format-number($jobId, ...) therefore received an empty node-set and, with
//     the old NaN-is-zero mistake, printed 0.00-padded numbers for any job id.
//
// Every expected value below was taken from xsltproc (libxslt 1.1.45) on the
// same input; the test data deliberately mirrors the REMITT shapes (a job id
// parameter, node-sets that may be empty, sums).

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/freemed/gokogiri/xml"
)

const formatNumberInput = `<remitt>` +
	`<num>150.5</num>` +
	`<empty></empty>` +
	`<text>abc</text>` +
	`<zero>0</zero>` +
	`</remitt>`

const formatNumberStylePrefix = `<?xml version="1.0"?>
<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
<xsl:output method="xml" omit-xml-declaration="yes"/>
<xsl:param name="jobId"/>
`

// nsAttrOverride strips the namespace declarations the engine copies from the
// stylesheet onto the output root element: their content is covered by other
// tests and their order comes from map iteration.
var nsAttrOverride = regexp.MustCompile(`\s+xmlns(:[A-Za-z0-9_.\-]+)?="[^"]*"`)

// transformFormatNumber runs one stylesheet body through the production entry
// points with a jobId parameter, the way remitt-server/common does.
func transformFormatNumber(t *testing.T, body, params string) string {
	t.Helper()
	dir := t.TempDir()
	xslFile := filepath.Join(dir, "format-number.xsl")
	inFile := filepath.Join(dir, "format-number.xml")
	if err := os.WriteFile(xslFile, []byte(formatNumberStylePrefix+body+
		"</xsl:stylesheet>\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inFile, []byte(formatNumberInput), 0644); err != nil {
		t.Fatal(err)
	}
	style, err := xml.ReadFile(xslFile, xml.StrictParseOption)
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	doc, err := xml.ReadFile(inFile, xml.StrictParseOption)
	if err != nil {
		t.Fatalf("read input document: %v", err)
	}
	ss, err := ParseStylesheet(style, xslFile)
	if err != nil {
		t.Fatalf("parse stylesheet: %v", err)
	}
	options := StylesheetOptions{}
	if params != "" {
		options.Parameters = map[string]interface{}{"jobId": params}
	}
	out, err := ss.Process(doc, options)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	return strings.TrimSpace(nsAttrOverride.ReplaceAllString(out, ""))
}

// TestFormatNumberPatterns pins the pattern semantics that the REMITT
// stylesheets depend on, including the '#.00' family that renders ".00" for a
// zero value. Expected values are xsltproc's.
func TestFormatNumberPatterns(t *testing.T) {
	cases := []struct {
		name     string
		expr     string
		expected string
	}{
		// The interchange control number: a 9-zero pattern over the job id.
		{"nine-zero-pad", "format-number($jobId, '000000000')", "000000001"},
		{"nine-zero-pad-literal", "format-number(1, '000000000')", "000000001"},
		{"nine-zero-pad-zero", "format-number(0, '000000000')", "000000000"},
		{"nine-zero-pad-rounds-up", "format-number(0.5, '000000000')", "000000001"},

		// '#' is an optional digit, '0' a mandatory one.
		{"hash-only-integer-part-suppressed", "format-number(0, '#.00')", ".00"},
		{"multi-hash-suppressed", "format-number(0, '####.00')", ".00"},
		{"mandatory-zero-kept", "format-number(0, '0.00')", "0.00"},
		{"hash-only-number", "format-number(0, '#')", "0"},
		{"hash-only-rounds-up", "format-number(0.5, '#')", "1"},
		{"hash-only-fraction-dropped", "format-number(0, '#.##')", "0"},

		// Rounding happens before the integer/fraction split.
		{"carry-into-integer", "format-number(2.999, '0.00')", "3.00"},

		// Grouping is requested by the pattern itself.
		{"grouping", "format-number(1234.5678, '#,##0.00')", "1,234.57"},
		{"grouping-millions", "format-number(1000000, '#,##0.00')", "1,000,000.00"},

		// Prefixes and suffixes are literal text.
		{"prefix", "format-number(150.5, '$0.00')", "$150.50"},

		// Signs.
		{"negative", "format-number(-3, '#.00')", "-3.00"},
		{"negative-optional-integer", "format-number(-0.25, '#.00')", "-.25"},
		{"negative-zero", "format-number(-0.25, '#')", "-0"},

		// Special values.
		{"infinity", "format-number(1 div 0, '0.00')", "Infinity"},
		{"minus-infinity", "format-number(-1 div 0, '0.00')", "-Infinity"},
		{"nan", "format-number(0 div 0, '0.00')", "NaN"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			body := `<xsl:template match="/remitt"><out><v><xsl:value-of select="` +
				tc.expr + `"/></v></out></xsl:template>`
			got := transformFormatNumber(t, body, "1")
			expected := "<out><v>" + tc.expected + "</v></out>"
			if got != expected {
				t.Errorf("%s\n  got:      %s\n  expected: %s", tc.expr, got, expected)
			}
		})
	}
}

// TestFormatNumberNodeSetArgument covers the XPath 1.0 number() conversion of
// the first argument: a node-set contributes the string-value of its first
// node, an EMPTY node-set is NaN (this is the "0.00" vs "NaN" divergence in
// cms1500 box 24f and the statement charge columns), and so is a node whose
// string-value or a literal string that is not a valid XPath number.
func TestFormatNumberNodeSetArgument(t *testing.T) {
	cases := []struct {
		name     string
		expr     string
		expected string
	}{
		{"missing-element", "format-number(//missing, '0.00')", "NaN"},
		{"empty-element", "format-number(//empty, '0.00')", "NaN"},
		{"non-numeric-text", "format-number(//text, '0.00')", "NaN"},
		{"numeric-element", "format-number(//num, '0.00')", "150.50"},
		{"zero-element", "format-number(//zero, '0.00')", "0.00"},
		{"optional-pattern-from-element", "format-number(//zero, '#.00')", ".00"},
		{"numeric-string", "format-number('12.5', '0.00')", "12.50"},
		{"non-numeric-string", "format-number('abc', '0.00')", "NaN"},
		{"empty-string", "format-number('', '0.00')", "NaN"},
		{"sum-of-numbers", "format-number(sum(//num), '0.00')", "150.50"},
		{"sum-of-empty-node-set-is-zero", "format-number(sum(//missing), '0.00')", "0.00"},
		{"zero-minus-zero", "format-number(0 - 0, '0.00')", "0.00"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			body := `<xsl:template match="/remitt"><out><v><xsl:value-of select="` +
				tc.expr + `"/></v></out></xsl:template>`
			got := transformFormatNumber(t, body, "1")
			expected := "<out><v>" + tc.expected + "</v></out>"
			if got != expected {
				t.Errorf("%s\n  got:      %s\n  expected: %s", tc.expr, got, expected)
			}
		})
	}
}

// TestFormatNumberJobIdParameter is the end-to-end shape of the 837P
// interchange control number: a global variable that formats a Go-supplied
// parameter. It fails if the parameter is not bound before global variables are
// evaluated, if scalar arguments to extension functions are dropped, or if the
// all-zero pattern is split at the wrong character.
func TestFormatNumberJobIdParameter(t *testing.T) {
	body := `<xsl:variable name="interchangeControlNumber" select="format-number($jobId, '000000000')"/>` +
		`<xsl:template match="/remitt"><out>` +
		`<ctl><xsl:value-of select="$interchangeControlNumber"/></ctl>` +
		`<direct><xsl:value-of select="$jobId"/></direct>` +
		`</out></xsl:template>`

	got := transformFormatNumber(t, body, "1")
	if expected := "<out><ctl>000000001</ctl><direct>1</direct></out>"; got != expected {
		t.Errorf("job id parameter\n  got:      %s\n  expected: %s", got, expected)
	}

	got = transformFormatNumber(t, body, "987654321")
	if expected := "<out><ctl>987654321</ctl><direct>987654321</direct></out>"; got != expected {
		t.Errorf("job id parameter\n  got:      %s\n  expected: %s", got, expected)
	}
}

// TestParseXPathNumber pins the XPath 1.0 string-to-number rules (spec 4.4)
// directly, including the cases that used to be swallowed by strconv's zero
// value on error.
func TestParseXPathNumber(t *testing.T) {
	cases := []struct {
		in       string
		expected float64
		nan      bool
	}{
		{in: "1", expected: 1},
		{in: "-1", expected: -1},
		{in: "  42  ", expected: 42},
		{in: "1.5", expected: 1.5},
		{in: ".5", expected: 0.5},
		{in: "-.5", expected: -0.5},
		{in: "12.", expected: 12},
		{in: "0", expected: 0},
		{in: "", nan: true},
		{in: "   ", nan: true},
		{in: "abc", nan: true},
		{in: "1e5", nan: true},  // no exponent notation in XPath 1.0
		{in: "+1", nan: true},   // no unary plus in XPath 1.0
		{in: "0x10", nan: true}, // no hex
		{in: "1 2", nan: true},  // trailing garbage
		{in: "-", nan: true},    // sign without digits
		{in: "NaN", nan: true},  // NaN is a value, not a string form
		{in: "Inf", nan: true},  // likewise for infinity
	}

	for _, tc := range cases {
		got := parseXPathNumber(tc.in)
		if tc.nan {
			if got == got { // NaN != NaN
				t.Errorf("parseXPathNumber(%q) = %v, expected NaN", tc.in, got)
			}
			continue
		}
		if got != tc.expected {
			t.Errorf("parseXPathNumber(%q) = %v, expected %v", tc.in, got, tc.expected)
		}
	}
}

// TestXpathArgToNumber covers the argument shapes the XPath engine hands to
// XSLT functions: absent/nil arguments, scalars, and node-sets (including the
// empty one, which must be NaN rather than 0).
func TestXpathArgToNumber(t *testing.T) {
	if got := xpathArgToNumber(nil); got == got {
		t.Errorf("nil argument = %v, expected NaN", got)
	}
	if got := xpathArgToNumber(nilTypedSlice()); got == got {
		t.Errorf("empty node-set argument = %v, expected NaN", got)
	}
	if got := xpathArgToNumber(""); got == got {
		t.Errorf("empty string argument = %v, expected NaN", got)
	}
	if got := xpathArgToNumber([]interface{}{"7.25"}); got != 7.25 {
		t.Errorf("scalar-wrapped argument = %v, expected 7.25", got)
	}
	if got := xpathArgToNumber(true); got != 1 {
		t.Errorf("true = %v, expected 1", got)
	}
	if got := xpathArgToNumber(false); got != 0 {
		t.Errorf("false = %v, expected 0", got)
	}
	if got := xpathArgToNumber(42); got != 42 {
		t.Errorf("int = %v, expected 42", got)
	}
	if got := xpathArgToNumber(4.5); got != 4.5 {
		t.Errorf("float64 = %v, expected 4.5", got)
	}
}

// nilTypedSlice returns an argument of the shape an empty node-set arrives in
// (the engine's function-argument conversion produces a nil []interface{}).
func nilTypedSlice() []interface{} {
	return nil
}

// TestSplitNumberPattern pins the pattern grammar the formatting depends on:
// the decimal separator only separates the two parts when it is really the
// decimal separator character, and everything before/after the number is
// literal prefix/suffix text.
func TestSplitNumberPattern(t *testing.T) {
	cases := []struct {
		format                            string
		prefix, intPart, fracPart, suffix string
		hasDecimal                        bool
	}{
		{format: "000000000", prefix: "", intPart: "000000000", fracPart: "", suffix: ""},
		{format: "#0.00", intPart: "#0", fracPart: "00", hasDecimal: true},
		{format: "#.00", intPart: "#", fracPart: "00", hasDecimal: true},
		{format: "0", intPart: "0"},
		{format: "1", prefix: "1", intPart: "", suffix: ""},
		{format: "$#,##0.00", prefix: "$", intPart: "#,##0", fracPart: "00", hasDecimal: true},
		{format: "0.00 USD", intPart: "0", fracPart: "00", hasDecimal: true, suffix: " USD"},
		{format: "0.", intPart: "0", hasDecimal: true},
	}
	for _, tc := range cases {
		prefix, intPart, fracPart, suffix, hasDecimal := splitNumberPattern(tc.format, ".")
		if prefix != tc.prefix || intPart != tc.intPart || fracPart != tc.fracPart ||
			suffix != tc.suffix || hasDecimal != tc.hasDecimal {
			t.Errorf("splitNumberPattern(%q) = (%q, %q, %q, %q, %v), expected (%q, %q, %q, %q, %v)",
				tc.format, prefix, intPart, fracPart, suffix, hasDecimal,
				tc.prefix, tc.intPart, tc.fracPart, tc.suffix, tc.hasDecimal)
		}
	}
}
