package xslt

// Regression tests for the EXSLT set functions (http://exslt.org/sets), which
// the shipped REMITT stylesheets (resources/xsl/cms1500.xsl, statement.xsl)
// use to collapse repeated nodes.
//
// Three defects these tests guard against:
//
//  1. The node-set argument handed to an extension function used to be a slice
//     of N aliases of a single NodeNavigator: antchfx's query.Select
//     implementations reuse and mutate one navigator, so by the time the
//     resolver consumed the collected arguments every element pointed at
//     whatever node the traversal had been rewound to (the document root for
//     `//practice`). Every string-value was therefore identical and
//     set:distinct() collapsed everything to one node. Fixed in the XPath
//     engine (collectNodes snapshots each navigator).
//  2. Attribute results (//practice/@id) were dropped by the argument
//     conversion, because the XPath layer hands attributes over as throw-away
//     *xpath.AttrNode copies that cannot be stored in the []*xml.InternalNode
//     the conversion produced. They are now materialised as DOM attribute
//     nodes carrying their owning element.
//  3. The surviving node used to be a node with no attributes attached, so even
//     the one <p> that was emitted came out empty.
//
// The assertions deliberately check the ATTRIBUTE VALUES that end up in the
// output, not just how many nodes come back: an implementation that returns the
// right number of the wrong nodes renders empty <p/> elements and would pass a
// count-only assertion.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/freemed/gokogiri/xml"
)

// Input documents. The duplicate pair has identical text content (which
// set:distinct() must collapse) but different ids, so a test can tell which
// node survived.
const (
	setTestInput          = `<remitt><practice id="1"><name>A</name></practice><practice id="2"><name>B</name></practice></remitt>`
	setTestDuplicateInput = `<remitt><practice id="1"><name>A</name></practice><practice id="2"><name>A</name></practice><practice id="3"><name>B</name></practice></remitt>`
	// setTestClaimInput mirrors the cms1500.xsl shape: a flat list of records
	// whose key children are deduplicated per practice.
	setTestClaimInput = `<claim>` +
		`<procedure><practicekey>P1</practicekey><providerkey>K1</providerkey></procedure>` +
		`<procedure><practicekey>P1</practicekey><providerkey>K1</providerkey></procedure>` +
		`<procedure><practicekey>P1</practicekey><providerkey>K2</providerkey></procedure>` +
		`<procedure><practicekey>P2</practicekey><providerkey>K3</providerkey></procedure>` +
		`</claim>`
)

const setTestStylesheetPrefix = `<?xml version="1.0"?>
<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform" xmlns:set="http://exslt.org/sets" xmlns:exsl="http://exslt.org/common">
<xsl:output method="xml" omit-xml-declaration="yes"/>
`

// namespaceDecl matches the namespace declarations the engine propagates from
// the stylesheet onto the output root element.
var namespaceDecl = regexp.MustCompile(`\s+xmlns(:[A-Za-z0-9_.\-]+)?="[^"]*"`)

// transformSets runs one stylesheet body against one input document through the
// production entry points (ParseStylesheet + Process) and returns the serialized
// result with the output root's namespace declarations removed: their content is
// checked by other tests and their ORDER comes from map iteration.
func transformSets(t *testing.T, input, body string) string {
	t.Helper()
	dir := t.TempDir()
	xslFile := filepath.Join(dir, "sets.xsl")
	inFile := filepath.Join(dir, "sets.xml")
	if err := os.WriteFile(xslFile, []byte(setTestStylesheetPrefix+body+"</xsl:stylesheet>\n"), 0644); err != nil {
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
	out, err := ss.Process(doc, StylesheetOptions{})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	return strings.TrimSpace(namespaceDecl.ReplaceAllString(out, ""))
}

// TestExsltSetDistinct covers set:distinct() for element node-sets, attribute
// node-sets, and the exsl:node-set($var/@attr) argument shape cms1500.xsl uses.
func TestExsltSetDistinct(t *testing.T) {
	cases := []struct {
		name  string
		input string
		body  string
		want  string
	}{
		{
			// The headline case: two nodes with different string-values are
			// both kept, and each one is the real DOM node (its @id is
			// reachable from inside the for-each body).
			name:  "distinct-nodes-with-different-text",
			input: setTestInput,
			body:  `<xsl:template match="/remitt"><out><xsl:for-each select="set:distinct(//practice)"><p><xsl:value-of select="@id"/></p></xsl:for-each></out></xsl:template>`,
			want:  `<out><p>1</p><p>2</p></out>`,
		},
		{
			// Dedup semantics: two practice elements with identical text
			// content collapse to one node, and it is the FIRST one.
			name:  "identical-text-collapses-to-first-node",
			input: setTestDuplicateInput,
			body:  `<xsl:template match="/remitt"><out n="{count(set:distinct(//practice))}"><xsl:for-each select="set:distinct(//practice)"><p><xsl:value-of select="@id"/></p></xsl:for-each></out></xsl:template>`,
			want:  `<out n="2"><p>1</p><p>3</p></out>`,
		},
		{
			// Attribute node-set: distinct values are the attribute VALUES,
			// even when the owning elements have identical text.
			name:  "distinct-attribute-nodes",
			input: setTestDuplicateInput,
			body:  `<xsl:template match="/remitt"><out><xsl:for-each select="set:distinct(//practice/@id)"><p><xsl:value-of select="."/></p></xsl:for-each></out></xsl:template>`,
			want:  `<out><p>1</p><p>2</p><p>3</p></out>`,
		},
		{
			// Repeated attribute values collapse, and the survivors are still
			// attribute nodes (value reachable with select=".").
			name:  "identical-attribute-values-collapse",
			input: `<remitt><practice id="7"><name>A</name></practice><practice id="7"><name>B</name></practice><practice id="9"><name>C</name></practice></remitt>`,
			body:  `<xsl:template match="/remitt"><out n="{count(set:distinct(//practice/@id))}"><xsl:for-each select="set:distinct(//practice/@id)"><p><xsl:value-of select="."/></p></xsl:for-each></out></xsl:template>`,
			want:  `<out n="2"><p>7</p><p>9</p></out>`,
		},
		{
			// Element nodes with identical text collapse (the whole point of
			// the function) - two <name>A</name> and one <name>B</name>.
			name:  "identical-element-text-collapses",
			input: setTestDuplicateInput,
			body:  `<xsl:template match="/remitt"><out><xsl:for-each select="set:distinct(//practice/name)"><p><xsl:value-of select="."/></p></xsl:for-each></out></xsl:template>`,
			want:  `<out><p>A</p><p>B</p></out>`,
		},
		{
			name:  "single-node-node-set-is-returned",
			input: setTestInput,
			body:  `<xsl:template match="/remitt"><out n="{count(set:distinct(//practice[@id='2']))}"><xsl:for-each select="set:distinct(//practice[@id='2'])"><p><xsl:value-of select="@id"/></p></xsl:for-each></out></xsl:template>`,
			want:  `<out n="1"><p>2</p></out>`,
		},
		{
			name:  "empty-node-set-stays-empty",
			input: setTestInput,
			body:  `<xsl:template match="/remitt"><out n="{count(set:distinct(//nope))}"><xsl:for-each select="set:distinct(//nope)"><p/></xsl:for-each></out></xsl:template>`,
			want:  `<out n="0"/>`,
		},
		{
			// A non-node-set argument is a type error; it must not panic and
			// must produce an empty node-set (not a synthesized one-node set).
			name:  "string-argument-does-not-panic",
			input: setTestInput,
			body:  `<xsl:template match="/remitt"><out n="{count(set:distinct('hello'))}"><xsl:for-each select="set:distinct('hello')"><p>unexpected</p></xsl:for-each></out></xsl:template>`,
			want:  `<out n="0"/>`,
		},
		{
			name:  "number-argument-does-not-panic",
			input: setTestInput,
			body:  `<xsl:template match="/remitt"><out n="{count(set:distinct(1))}"><xsl:for-each select="set:distinct(1)"><p>unexpected</p></xsl:for-each></out></xsl:template>`,
			want:  `<out n="0"/>`,
		},
		{
			// cms1500.xsl argument shape #1: exsl:node-set($var/@attr).
			name:  "exsl-node-set-of-variable-attribute",
			input: setTestDuplicateInput,
			body:  `<xsl:template match="/remitt"><xsl:variable name="p" select="//practice"/><out><xsl:for-each select="set:distinct(exsl:node-set($p/@id))"><p><xsl:value-of select="."/></p></xsl:for-each></out></xsl:template>`,
			want:  `<out><p>1</p><p>2</p><p>3</p></out>`,
		},
		{
			// cms1500.xsl argument shape #2: exsl:node-set($var) over elements.
			name:  "exsl-node-set-of-variable-elements",
			input: setTestDuplicateInput,
			body:  `<xsl:template match="/remitt"><xsl:variable name="p" select="//practice"/><out><xsl:for-each select="set:distinct(exsl:node-set($p))"><p><xsl:value-of select="@id"/></p></xsl:for-each></out></xsl:template>`,
			want:  `<out><p>1</p><p>3</p></out>`,
		},
		{
			// The result of set:distinct() must itself behave like a node-set
			// in an XPath expression (count() and a positional filter).
			// NOTE: a numeric predicate evaluated directly on an extension
			// function result - set:distinct(//practice)[1] - is a separate,
			// pre-existing limitation of the XPath engine (it is ignored for
			// exsl:node-set(...) too); the positional form works and is what
			// the shipped stylesheets use.
			name:  "result-usable-as-node-set",
			input: setTestInput,
			body:  `<xsl:template match="/remitt"><out n="{count(set:distinct(//practice))}"><xsl:for-each select="set:distinct(//practice)[position()=1]"><p><xsl:value-of select="@id"/></p></xsl:for-each></out></xsl:template>`,
			want:  `<out n="2"><p>1</p></out>`,
		},
		{
			// Predicate inside the argument (cms1500.xsl wraps its own
			// set:distinct around $nodes[position() > 1]).
			name:  "predicate-inside-argument",
			input: setTestInput,
			body:  `<xsl:template match="/remitt"><out n="{count(set:distinct(//practice[position() &gt; 1]))}"><xsl:for-each select="set:distinct(//practice[position() &gt; 1])"><p><xsl:value-of select="@id"/></p></xsl:for-each></out></xsl:template>`,
			want:  `<out n="1"><p>2</p></out>`,
		},
		{
			// The cms1500.xsl idiom verbatim: deduplicate a key child of the
			// records selected by a variable in a predicate, bind the result
			// to a variable, and iterate that variable.
			name:  "cms1500-provider-dedup-idiom",
			input: setTestClaimInput,
			body: `<xsl:template match="/claim"><xsl:variable name="practice" select="'P1'"/>` +
				`<xsl:variable name="providers" select="set:distinct(exsl:node-set(//procedure[practicekey=$practice]/providerkey))"/>` +
				`<out n="{count($providers)}"><xsl:for-each select="$providers"><p><xsl:value-of select="."/></p></xsl:for-each></out></xsl:template>`,
			want: `<out n="2"><p>K1</p><p>K2</p></out>`,
		},
		{
			// statement.xsl passes set:distinct() results around as variables
			// and template parameters; iteration from the parameter must see
			// the same nodes.
			name:  "result-passed-as-template-parameter",
			input: setTestClaimInput,
			body: `<xsl:template match="/claim"><out><xsl:call-template name="show"><xsl:with-param name="diags" select="set:distinct(//procedure/providerkey)"/></xsl:call-template></out></xsl:template>` +
				`<xsl:template name="show"><xsl:param name="diags"/><n><xsl:value-of select="count($diags)"/></n><xsl:for-each select="$diags"><p><xsl:value-of select="."/></p></xsl:for-each></xsl:template>`,
			want: `<out><n>3</n><p>K1</p><p>K2</p><p>K3</p></out>`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := transformSets(t, tc.input, tc.body)
			if got != tc.want {
				t.Errorf("output mismatch\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

// TestExsltSetSiblingFunctions covers the other functions in the same family,
// which share set:distinct()'s node-set argument handling and identity rules.
//
// set:has-same-node() is asserted through xsl:value-of rather than
// xsl:if/@test: a boolean result coming back from an extension function is
// wrapped into a one-element node-set by the XPath engine, so it is always
// true in a boolean context (a pre-existing engine limitation that applies to
// every boolean-returning extension function, not just this one). value-of
// reads the real result.
func TestExsltSetSiblingFunctions(t *testing.T) {
	cases := []struct {
		name  string
		input string
		body  string
		want  string
	}{
		{
			name:  "difference-drops-nodes-of-second-set",
			input: setTestInput,
			body:  `<xsl:template match="/remitt"><out><xsl:for-each select="set:difference(//practice, //practice[@id='1'])"><p><xsl:value-of select="@id"/></p></xsl:for-each></out></xsl:template>`,
			want:  `<out><p>2</p></out>`,
		},
		{
			name:  "difference-by-attribute-value",
			input: setTestInput,
			body:  `<xsl:template match="/remitt"><out><xsl:for-each select="set:difference(//practice/@id, //practice[@id='2']/@id)"><p><xsl:value-of select="."/></p></xsl:for-each></out></xsl:template>`,
			want:  `<out><p>1</p></out>`,
		},
		{
			name:  "intersection-keeps-shared-nodes",
			input: setTestInput,
			body:  `<xsl:template match="/remitt"><out><xsl:for-each select="set:intersection(//practice, //practice[@id='2'])"><p><xsl:value-of select="@id"/></p></xsl:for-each></out></xsl:template>`,
			want:  `<out><p>2</p></out>`,
		},
		{
			name:  "intersection-by-attribute",
			input: setTestInput,
			body:  `<xsl:template match="/remitt"><out><xsl:for-each select="set:intersection(//practice/@id, //practice[@id='1']/@id)"><p><xsl:value-of select="."/></p></xsl:for-each></out></xsl:template>`,
			want:  `<out><p>1</p></out>`,
		},
		{
			name:  "has-same-node",
			input: setTestInput,
			body:  `<xsl:template match="/remitt"><out a="{set:has-same-node(//practice, //practice[@id='2'])}" b="{set:has-same-node(//practice, //nope)}" c="{set:has-same-node(//practice/@id, //practice[@id='1']/@id)}" d="{set:has-same-node(//practice/@id, //practice[@id='1']/name)}"/></xsl:template>`,
			want:  `<out a="true" b="false" c="true" d="false"/>`,
		},
		{
			name:  "leading-and-trailing-split-at-first-node-of-second-set",
			input: setTestInput,
			body:  `<xsl:template match="/remitt"><out><l><xsl:for-each select="set:leading(//practice, //practice[@id='2'])"><p><xsl:value-of select="@id"/></p></xsl:for-each></l><t><xsl:for-each select="set:trailing(//practice, //practice[@id='1'])"><p><xsl:value-of select="@id"/></p></xsl:for-each></t></out></xsl:template>`,
			want:  `<out><l><p>1</p></l><t><p>2</p></t></out>`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := transformSets(t, tc.input, tc.body)
			if got != tc.want {
				t.Errorf("output mismatch\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}
