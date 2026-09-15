package xslt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/freemed/gokogiri/xml"
)

// Regression tests for node-set variables: $var bound to a node set must
// stay a node set through xsl:for-each, xsl:call-template/xsl:with-param,
// global variables and predicates.
//
// The defect these guard against: instructions compiled their @select/@test
// with xpath.Compile(), which runs without the XSLT variable/function
// resolvers and therefore returns nil for any expression containing a
// $variable reference. The nil expression then evaluated to an empty result,
// so `for-each select="$nodeset"` silently produced nothing (and so did
// copy-of / apply-templates @select / xsl:if @test / xsl:choose @test).
//
// A second defect: a node-set variable resolved to the string "" when it was
// empty, and to a bare navigator (rather than a slice) when it held exactly
// one node. The empty case iterated once instead of zero times (count() == 1),
// and the 1-node case took a different code path than the N-node case.

// input document for every case below.
const variableTestInput = `<remitt><practice id="1"><name>A</name></practice><practice id="2"><name>B</name></practice></remitt>`

const xslPrefix = `<?xml version="1.0"?>
<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
<xsl:output method="xml" omit-xml-declaration="yes"/>
`

// transformVariables runs the given stylesheet body against variableTestInput
// through the same API production uses: ParseStylesheet + Process.
func transformVariables(t *testing.T, xslBody string, params map[string]any, options StylesheetOptions) string {
	t.Helper()
	dir := t.TempDir()
	xslFile := filepath.Join(dir, "variables.xsl")
	inFile := filepath.Join(dir, "variables.xml")
	if err := os.WriteFile(xslFile, []byte(xslPrefix+xslBody+"</xsl:stylesheet>\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inFile, []byte(variableTestInput), 0644); err != nil {
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
	options.Parameters = params
	out, err := ss.Process(doc, options)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	return strings.TrimSpace(out)
}

func TestNodeSetVariableResolution(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		want   string
		params map[string]any
	}{
		// ---- working behaviour that must not regress ----
		{
			name: "literal-result-elements-count-and-literal-for-each",
			body: `<xsl:template match="/remitt"><out n="{count(//practice)}"><xsl:for-each select="//practice"><p><xsl:value-of select="@id"/>-<xsl:value-of select="name"/></p></xsl:for-each></out></xsl:template>`,
			want: `<out n="2"><p>1-A</p><p>2-B</p></out>`,
		},
		{
			name: "literal-predicate",
			body: `<xsl:template match="/remitt"><out><xsl:for-each select="//practice[@id='1']"><p><xsl:value-of select="name"/></p></xsl:for-each></out></xsl:template>`,
			want: `<out><p>A</p></out>`,
		},
		{
			name: "local-string-variable-in-value-of",
			body: `<xsl:template match="/remitt"><xsl:variable name="label" select="'LBL'"/><out><xsl:value-of select="$label"/></out></xsl:template>`,
			want: `<out>LBL</out>`,
		},
		{
			name:   "go-supplied-param-in-value-of",
			body:   `<xsl:param name="jobId"/><xsl:template match="/remitt"><out><xsl:value-of select="$jobId"/></out></xsl:template>`,
			want:   `<out>JOB42</out>`,
			params: map[string]any{"jobId": "JOB42"},
		},

		// ---- node-set variables ----
		{
			// count()/value-of already worked; they are the baseline that
			// proves the variable itself resolves, so if for-each breaks
			// again the failure is in iteration, not resolution.
			name: "count-and-value-of-node-set-variable",
			body: `<xsl:template match="/remitt"><xsl:variable name="p" select="//practice"/><out><n><xsl:value-of select="count($p)"/></n><f><xsl:value-of select="$p/name"/></f></out></xsl:template>`,
			want: `<out><n>2</n><f>A</f></out>`,
		},
		{
			name: "for-each-over-node-set-variable",
			body: `<xsl:template match="/remitt"><xsl:variable name="practices" select="//practice"/><out><xsl:for-each select="$practices"><p><xsl:value-of select="name"/></p></xsl:for-each></out></xsl:template>`,
			want: `<out><p>A</p><p>B</p></out>`,
		},
		{
			// 1-node node-set must behave exactly like the N-node one.
			name: "for-each-over-one-node-node-set-variable",
			body: `<xsl:template match="/remitt"><xsl:variable name="practices" select="//practice[@id='2']"/><out><xsl:for-each select="$practices"><p><xsl:value-of select="name"/></p></xsl:for-each></out></xsl:template>`,
			want: `<out><p>B</p></out>`,
		},
		{
			name: "position-and-last-inside-for-each-over-node-set-variable",
			body: `<xsl:template match="/remitt"><xsl:variable name="p" select="//practice"/><out><xsl:for-each select="$p"><p pos="{position()}" last="{last()}"><xsl:value-of select="@id"/></p></xsl:for-each></out></xsl:template>`,
			want: `<out><p pos="1" last="2">1</p><p pos="2" last="2">2</p></out>`,
		},
		{
			name: "with-param-node-set-variable",
			body: `<xsl:template match="/remitt"><out><xsl:call-template name="show"><xsl:with-param name="ns" select="//practice"/></xsl:call-template></out></xsl:template>` +
				`<xsl:template name="show"><xsl:param name="ns"/><xsl:for-each select="$ns"><p><xsl:value-of select="name"/></p></xsl:for-each></xsl:template>`,
			want: `<out><p>A</p><p>B</p></out>`,
		},
		{
			name: "global-node-set-variable",
			body: `<xsl:variable name="allp" select="//practice"/><xsl:template match="/remitt"><out><xsl:for-each select="$allp"><p><xsl:value-of select="name"/></p></xsl:for-each></out></xsl:template>`,
			want: `<out><p>A</p><p>B</p></out>`,
		},
		{
			name: "scalar-variable-in-predicate",
			body: `<xsl:template match="/remitt"><xsl:variable name="want" select="'1'"/><out><xsl:for-each select="//practice[@id=$want]"><p><xsl:value-of select="name"/></p></xsl:for-each></out></xsl:template>`,
			want: `<out><p>A</p></out>`,
		},
		{
			name: "node-set-variable-in-predicate",
			body: `<xsl:template match="/remitt"><xsl:variable name="want" select="//practice[@id='2']"/><out><xsl:for-each select="//practice[@id=$want/@id]"><p><xsl:value-of select="name"/></p></xsl:for-each></out></xsl:template>`,
			want: `<out><p>B</p></out>`,
		},
		{
			// An empty node-set is a node-set: zero iterations, count() == 0.
			name: "empty-node-set-variable",
			body: `<xsl:template match="/remitt"><xsl:variable name="none" select="//nope"/><out n="{count($none)}"><xsl:for-each select="$none"><p/></xsl:for-each></out></xsl:template>`,
			want: `<out n="0"/>`,
		},
		{
			name: "copy-of-node-set-variable",
			body: `<xsl:template match="/remitt"><xsl:variable name="p" select="//practice"/><out><xsl:copy-of select="$p"/></out></xsl:template>`,
			want: `<out><practice id="1"><name>A</name></practice><practice id="2"><name>B</name></practice></out>`,
		},
		{
			name: "apply-templates-select-node-set-variable",
			body: `<xsl:template match="/remitt"><xsl:variable name="p" select="//practice"/><out><xsl:apply-templates select="$p"/></out></xsl:template>` +
				`<xsl:template match="practice"><x><xsl:value-of select="name"/></x></xsl:template>`,
			want: `<out><x>A</x><x>B</x></out>`,
		},
		{
			name: "if-test-on-node-set-variable",
			body: `<xsl:template match="/remitt"><xsl:variable name="p" select="//practice"/><xsl:variable name="none" select="//nope"/><out><xsl:if test="$p/name='B'"><hasB/></xsl:if><xsl:if test="$none"><unexpected/></xsl:if></out></xsl:template>`,
			want: `<out><hasB/></out>`,
		},
		{
			// A single-node variable (current node) used as the selection of a
			// nested for-each, then a path step from a variable inside the loop.
			name: "nested-for-each-over-node-set-variables",
			body: `<xsl:template match="/remitt"><xsl:variable name="p" select="//practice"/><out><xsl:for-each select="$p"><row><xsl:variable name="self" select="."/><xsl:for-each select="$self/name"><xsl:value-of select="."/></xsl:for-each></row></xsl:for-each></out></xsl:template>`,
			want: `<out><row>A</row><row>B</row></out>`,
		},
		{
			// The idiom the shipped REMITT stylesheets are built on: for-each
			// over a node-set variable, with a second node-set variable
			// declared and iterated inside the loop.
			name: "remitt-idiom-nested-variables",
			body: `<xsl:template match="/remitt"><xsl:variable name="practices" select="//practice"/><out><xsl:for-each select="$practices"><xsl:variable name="names" select="name"/><p id="{@id}"><xsl:for-each select="$names"><xsl:value-of select="."/></xsl:for-each></p></xsl:for-each></out></xsl:template>`,
			want: `<out><p id="1">A</p><p id="2">B</p></out>`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := transformVariables(t, tc.body, tc.params, StylesheetOptions{})
			if got != tc.want {
				t.Errorf("output mismatch\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

// TestNodeSetVariableResolutionIndented runs the two headline cases with the
// production option shape (IndentOutput + Parameters), which is how
// REMITT calls the engine.
func TestNodeSetVariableResolutionIndented(t *testing.T) {
	body := `<xsl:param name="jobId"/>` +
		`<xsl:variable name="allp" select="//practice"/>` +
		`<xsl:template match="/remitt"><out job="{$jobId}"><xsl:for-each select="$allp"><p><xsl:value-of select="name"/></p></xsl:for-each></out></xsl:template>`
	want := "<out job=\"JOB42\">\n" +
		"  <p>\n    A\n  </p>\n" +
		"  <p>\n    B\n  </p>\n" +
		"</out>"
	got := transformVariables(t, body, map[string]any{"jobId": "JOB42"}, StylesheetOptions{IndentOutput: true})
	if got != want {
		t.Errorf("output mismatch\n got: %q\nwant: %q", got, want)
	}
}
