package xslt

// Regression tests for the XSLT context position (position(), last(), and the
// current() function) and for the order in which the caller's parameters are
// bound.
//
// Defect these tests guard against: the XPath engine has no notion of the XSLT
// "current node list", so its position() walks the preceding nodes of the DOM
// tree instead of returning the index of the current node in the node list
// being processed. That is only accidentally right when the node list is a run
// of adjacent siblings. statement.xsl renders each procedure row as
// `$line + $offset` with $line a position() taken in the enclosing
// xsl:for-each, so with a single procedure listed after ten other elements the
// row came out as 42 where xsltproc (libxslt, which tracks the node list)
// renders 19 -- the same shift appeared on every row of the statement. It also
// broke the 837P HL segment numbers (`$insuredhlrel + $insuredoffset`, printed
// as "21x1" instead of "2x1") and the BK/BF diagnosis qualifier in the HI
// segment, which is chosen by `position() = 1`.
//
// ratago now substitutes the position and size it already tracks for the
// current node list (SetContextPosition is called by xsl:for-each,
// xsl:apply-templates and the match-pattern evaluator) into the top-level
// position()/last() calls of an expression; inside a predicate the engine
// builds its own node list and is left alone.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/freemed/gokogiri/xml"
)

const contextPositionInput = `<remitt>` +
	`<practice id="1"><name>A</name></practice>` +
	`<practice id="2"><name>B</name></practice>` +
	`<patient id="1"><name>P1</name></patient>` +
	`<patient id="2"><name>P2</name></patient>` +
	`</remitt>`

// contextPositionClaimsInput mirrors the statement.xsl shape: one procedure
// that is NOT the first child, so a DOM-walking position() cannot pass by
// accident, plus a second one to exercise last().
const contextPositionClaimsInput = `<remitt>` +
	`<global><currentdate><year>2024</year><month>08</month><day>10</day></currentdate></global>` +
	`<practice id="1"><name>A</name></practice>` +
	`<provider id="1"><name>PROVIDER</name></provider>` +
	`<patient id="1"><name>P1</name></patient>` +
	`<procedure id="1"><patientkey>1</patientkey><diagnosiskey>1</diagnosiskey><charge>150.00</charge></procedure>` +
	`<procedure id="2"><patientkey>1</patientkey><diagnosiskey>1</diagnosiskey><charge>25.50</charge></procedure>` +
	`<diagnosis id="1"><icd9code>E11.9</icd9code></diagnosis>` +
	`</remitt>`

const contextPositionStylePrefix = `<?xml version="1.0"?>
<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
<xsl:output method="xml" omit-xml-declaration="yes"/>
`

// transformPosition runs one stylesheet body against one input document
// through the production entry points and returns the serialized result with
// the copied namespace declarations removed.
func transformPosition(t *testing.T, input, body string) string {
	t.Helper()
	return transformPositionWithParams(t, input, body, nil)
}

func transformPositionWithParams(t *testing.T, input, body string, params map[string]interface{}) string {
	t.Helper()
	dir := t.TempDir()
	xslFile := filepath.Join(dir, "position.xsl")
	inFile := filepath.Join(dir, "position.xml")
	if err := os.WriteFile(xslFile, []byte(contextPositionStylePrefix+body+
		"</xsl:stylesheet>\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inFile, []byte(input), 0644); err != nil {
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
	out, err := ss.Process(doc, StylesheetOptions{Parameters: params})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	nsDecl := regexp.MustCompile(`\s+xmlns(:[A-Za-z0-9_.\-]+)?="[^"]*"`)
	return strings.TrimSpace(nsDecl.ReplaceAllString(out, ""))
}

// TestPositionAndLastInForEach checks that position() and last() report the
// index and size of the node list the xsl:for-each is processing, not the
// position of the node in the document.
func TestPositionAndLastInForEach(t *testing.T) {
	body := `<xsl:template match="/remitt"><out>` +
		`<xsl:for-each select="//practice">` +
		`<p n="{position()}" of="{last()}"><xsl:value-of select="name"/></p>` +
		`</xsl:for-each>` +
		`</out></xsl:template>`

	got := transformPosition(t, contextPositionInput, body)
	expected := `<out><p n="1" of="2">A</p><p n="2" of="2">B</p></out>`
	if got != expected {
		t.Errorf("position() in for-each\n  got:      %s\n  expected: %s", got, expected)
	}
}

// TestPositionViaVariableAndCalledTemplate reproduces the statement.xsl row
// computation: position() is stored in a variable inside the for-each, passed
// to a named template as a parameter, and added to a constant offset there.
// With a procedure that is not the first child a DOM-walking position() yields
// a row far from 19.
func TestPositionViaVariableAndCalledTemplate(t *testing.T) {
	body := `<xsl:template match="/remitt"><out>` +
		`<xsl:call-template name="render-form"/>` +
		`</out></xsl:template>` +
		`<xsl:template name="render-form">` +
		`<xsl:param name="procs" select="//procedure[patientkey=1]"/>` +
		`<xsl:for-each select="$procs">` +
		`<xsl:variable name="thisproc" select="."/>` +
		`<xsl:variable name="pos" select="position()"/>` +
		`<xsl:call-template name="render-proc">` +
		`<xsl:with-param name="proc" select="$thisproc"/>` +
		`<xsl:with-param name="line" select="$pos"/>` +
		`</xsl:call-template>` +
		`</xsl:for-each>` +
		`</xsl:template>` +
		`<xsl:template name="render-proc">` +
		`<xsl:param name="proc"/>` +
		`<xsl:param name="line"/>` +
		`<xsl:variable name="offset" select="18"/>` +
		`<row><xsl:value-of select="($line + $offset)"/></row>` +
		`</xsl:template>`

	got := transformPosition(t, contextPositionClaimsInput, body)
	expected := `<out><row>19</row><row>20</row></out>`
	if got != expected {
		t.Errorf("statement row computation\n  got:      %s\n  expected: %s", got, expected)
	}
}

// TestPositionChoosesDiagnosisQualifier reproduces the 837P HI segment: a
// qualifier that depends on the position of the diagnosis reference in the
// list ("BK" for the first, "BF" for the others).
func TestPositionChoosesDiagnosisQualifier(t *testing.T) {
	body := `<xsl:template match="/remitt"><out>` +
		`<xsl:variable name="diags" select="//procedure[patientkey=1]/diagnosiskey"/>` +
		`<xsl:for-each select="$diags">` +
		`<q><xsl:choose>` +
		`<xsl:when test="position() = 1">BK</xsl:when>` +
		`<xsl:otherwise>BF</xsl:otherwise>` +
		`</xsl:choose></q>` +
		`</xsl:for-each>` +
		`</out></xsl:template>`

	got := transformPosition(t, contextPositionClaimsInput, body)
	expected := `<out><q>BK</q><q>BF</q></out>`
	if got != expected {
		t.Errorf("diagnosis qualifier\n  got:      %s\n  expected: %s", got, expected)
	}
}

// TestPositionInsidePredicateIsRelativeToPredicate is the guard on the other
// side of the substitution: position() inside a predicate belongs to the node
// list the engine is filtering, and must keep working.
func TestPositionInsidePredicateIsRelativeToPredicate(t *testing.T) {
	body := `<xsl:template match="/remitt"><out>` +
		`<first><xsl:value-of select="//practice[position() = 1]/name"/></first>` +
		`<second><xsl:value-of select="//practice[position() = 2]/name"/></second>` +
		`<notlast><xsl:value-of select="//practice[position() != last()]/name"/></notlast>` +
		`</out></xsl:template>`

	got := transformPosition(t, contextPositionInput, body)
	expected := `<out><first>A</first><second>B</second><notlast>A</notlast></out>`
	if got != expected {
		t.Errorf("position() inside predicate\n  got:      %s\n  expected: %s", got, expected)
	}
}

// TestPositionInApplyTemplates checks the same context for
// xsl:apply-templates, which also establishes a node list.
func TestPositionInApplyTemplates(t *testing.T) {
	body := `<xsl:template match="/remitt"><out>` +
		`<xsl:apply-templates select="//practice"/>` +
		`</out></xsl:template>` +
		`<xsl:template match="practice">` +
		`<p n="{position()}" of="{last()}"><xsl:value-of select="name"/></p>` +
		`</xsl:template>`

	got := transformPosition(t, contextPositionInput, body)
	expected := `<out><p n="1" of="2">A</p><p n="2" of="2">B</p></out>`
	if got != expected {
		t.Errorf("position() in apply-templates\n  got:      %s\n  expected: %s", got, expected)
	}
}

// TestCurrentFunctionInCalledTemplate reproduces cms1500.xsl's
// lookup-diagnosis: current() is evaluated inside an xsl:for-each to pass the
// node being iterated to a called template, whose body then compares
// current() with that node.
func TestCurrentFunctionInCalledTemplate(t *testing.T) {
	body := `<xsl:template match="/remitt"><out>` +
		`<xsl:variable name="diags" select="//procedure[patientkey=1]/diagnosiskey"/>` +
		`<xsl:for-each select="$diags">` +
		`<xsl:call-template name="lookup-diagnosis">` +
		`<xsl:with-param name="diag" select="current()"/>` +
		`<xsl:with-param name="set" select="//diagnosiskey"/>` +
		`</xsl:call-template>` +
		`</xsl:for-each>` +
		`</out></xsl:template>` +
		`<xsl:template name="lookup-diagnosis">` +
		`<xsl:param name="diag"/>` +
		`<xsl:param name="set"/>` +
		`<xsl:for-each select="$set">` +
		`<xsl:if test="current()=$diag"><ref><xsl:value-of select="position()"/></ref></xsl:if>` +
		`</xsl:for-each>` +
		`</xsl:template>`

	got := transformPosition(t, contextPositionClaimsInput, body)
	expected := `<out><ref>1</ref><ref>1</ref></out>`
	if got != expected {
		t.Errorf("current() in called template\n  got:      %s\n  expected: %s", got, expected)
	}
}

// TestParametersBoundBeforeGlobalVariables pins the order of the setup steps in
// Process: the values supplied through StylesheetOptions.Parameters must be
// visible to global variables. A global variable that formats a parameter (the
// 837P interchange control number) otherwise sees an unbound xsl:param, which
// the engine reports as a missing variable and then treats as an empty
// node-set.
func TestParametersBoundBeforeGlobalVariables(t *testing.T) {
	body := `<xsl:param name="jobId"/>` +
		`<xsl:variable name="interchangeControlNumber" select="format-number($jobId, '000000000')"/>` +
		`<xsl:variable name="plain" select="$jobId"/>` +
		`<xsl:template match="/remitt"><out>` +
		`<ctl><xsl:value-of select="$interchangeControlNumber"/></ctl>` +
		`<plain><xsl:value-of select="$plain"/></plain>` +
		`</out></xsl:template>`

	got := transformPositionWithParams(t, contextPositionInput, body,
		map[string]interface{}{"jobId": "42"})
	expected := `<out><ctl>000000042</ctl><plain>42</plain></out>`
	if got != expected {
		t.Errorf("parameters before global variables\n  got:      %s\n  expected: %s", got, expected)
	}
}

// TestRewriteContextPosition pins the expression rewriting itself: top-level
// calls are replaced by literals, calls inside a predicate, inside a string
// literal, or as part of a longer function name are not.
func TestRewriteContextPosition(t *testing.T) {
	cases := []struct {
		expr     string
		expected string
	}{
		{expr: "position()", expected: "3"},
		{expr: "last()", expected: "7"},
		{expr: "position() = last()", expected: "3 = 7"},
		{expr: "($pos - position()) * 2", expected: "($pos - 3) * 2"},
		{expr: "not(position()=last())", expected: "not(3=7)"},
		{expr: "position ()", expected: "3"},
		// Untouched: its own node list is built by the engine.
		{expr: "//practice[position() = 1]", expected: "//practice[position() = 1]"},
		{expr: "//practice[position() = last()]", expected: "//practice[position() = last()]"},
		{expr: "//practice[foo/bar[position()=1]]", expected: "//practice[foo/bar[position()=1]]"},
		// Untouched: data, not a call.
		{expr: "'position()'", expected: "'position()'"},
		{expr: "concat('last()', position())", expected: "concat('last()', 3)"},
		// Untouched: a different function with a similar name.
		{expr: "my:position()", expected: "my:position()"},
		{expr: "my-position()", expected: "my-position()"},
		{expr: "last-check()", expected: "last-check()"},
		// Argument list is not empty: not our function.
		{expr: "position(1)", expected: "position(1)"},
	}
	for _, tc := range cases {
		if got := rewriteContextPosition(tc.expr, 3, 7); got != tc.expected {
			t.Errorf("rewriteContextPosition(%q, 3, 7) = %q, expected %q",
				tc.expr, got, tc.expected)
		}
	}
	// Expressions without any context function must be returned unchanged --
	// EvalXPath relies on that to avoid recompiling.
	unchanged := "concat($a, 'b')"
	if got := rewriteContextPosition(unchanged, 3, 7); got != unchanged {
		t.Errorf("rewriteContextPosition(%q) = %q, expected it to be untouched", unchanged, got)
	}
}
