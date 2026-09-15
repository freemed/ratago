package xslt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/freemed/gokogiri/xml"
)

// Regression tests for XML serialization parity with xsltproc/libxslt.
//
// Two defects are covered here:
//
//  1. A carriage return produced by the stylesheet (the X12 CRLF segment
//     terminator in the REMITT stylesheets: <xsl:text>~&#x0D;&#x0A;</xsl:text>)
//     was written out raw. A raw CR inside XML content is normalized to a
//     linefeed by every conformant parser, so the CR was silently lost when
//     the consumer re-parsed the rendered XML and the EDI terminator degraded
//     from CRLF to LF. libxslt escapes it as &#13;; the output method must do
//     the same.
//
//  2. With indent="yes" the serializer padded text nodes
//     (<content>\n  A\n</content>) where libxslt writes <content>A</content>.
//     The REMITT fixed-form consumer slices field values straight out of
//     element text, so the injected newline and indentation became part of
//     the field value.

// transformStylesheet runs a complete stylesheet against an input document
// through the production API (ParseStylesheet + Process).
func transformStylesheet(t *testing.T, xsl, input string, params map[string]any, options StylesheetOptions) string {
	t.Helper()
	dir := t.TempDir()
	xslFile := filepath.Join(dir, "output.xsl")
	inFile := filepath.Join(dir, "input.xml")
	if err := os.WriteFile(xslFile, []byte(xsl), 0644); err != nil {
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
	out, err := ss.Process(doc, options)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	return out
}

const (
	serializeInput = `<remitt><global><billinguid>BILL-001</billinguid></global></remitt>`
	serializeXSL   = `<?xml version="1.0"?>
<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
<xsl:output method="xml" indent="yes"/>
<xsl:template match="/remitt">
<render>
<x12format>
<delimiter><xsl:text>*</xsl:text></delimiter>
<endofline><xsl:text>~&#x0D;&#x0A;</xsl:text></endofline>
</x12format>
<element>
<row>42</row>
<content><xsl:value-of select="global/billinguid"/></content>
</element>
</render>
</xsl:template>
</xsl:stylesheet>
`
)

// TestOutputEscapesCarriageReturn is the defect 1 guard: CR in text output is
// escaped as &#13; (and the linefeed that follows it stays literal), so that
// the CR survives re-parsing of the rendered document.
func TestOutputEscapesCarriageReturn(t *testing.T) {
	out := transformStylesheet(t, serializeXSL, serializeInput, nil, StylesheetOptions{IndentOutput: true})

	if strings.ContainsRune(out, '\r') {
		t.Errorf("output contains a raw carriage return, which a parser would normalize to LF:\n%q", out)
	}
	if !strings.Contains(out, "<endofline>~&#13;\n</endofline>") {
		t.Errorf("expected <endofline>~&#13;\\n</endofline>, got:\n%q", out)
	}

	// The functional check: re-parse the rendered XML and confirm the CR is
	// still there.
	doc, err := xml.Parse([]byte(out), xml.DefaultEncodingBytes, nil, xml.DefaultParseOption, xml.DefaultEncodingBytes)
	if err != nil {
		t.Fatalf("re-parse rendered output: %v", err)
	}
	nodes, err := doc.Root().Search("//endofline")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 endofline element, got %d", len(nodes))
	}
	if got := nodes[0].Content(); got != "~\r\n" {
		t.Errorf("round-tripped terminator: got %q, want %q (CRLF)", got, "~\r\n")
	}
}

// TestOutputDoesNotIndentTextContent is the defect 2 guard: with indent="yes"
// no whitespace is inserted inside an element's text content, matching
// libxslt, so verbatim consumers get exactly the field value.
func TestOutputDoesNotIndentTextContent(t *testing.T) {
	out := transformStylesheet(t, serializeXSL, serializeInput, nil, StylesheetOptions{IndentOutput: true})

	if !strings.Contains(out, "<content>BILL-001</content>") {
		t.Errorf("expected <content>BILL-001</content> (no injected padding), got:\n%q", out)
	}
	// Text-only elements exist, so elements with element children are still
	// indented on their own lines.
	for _, want := range []string{"<render>\n  <x12format>\n", "    <delimiter>*</delimiter>\n", "  <element>\n    <row>42</row>\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected indentation %q in:\n%q", want, out)
		}
	}
}

// TestOutputTextMethodIsVerbatim guards the "text" output method: it writes
// character data verbatim, so a CRLF stays a CRLF instead of becoming &#13;.
func TestOutputTextMethodIsVerbatim(t *testing.T) {
	xsl := `<?xml version="1.0"?>
<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
<xsl:output method="text"/>
<xsl:template match="/remitt"><xsl:text>billing=</xsl:text><xsl:value-of select="global/billinguid"/><xsl:text>~&#x0D;&#x0A;</xsl:text></xsl:template>
</xsl:stylesheet>
`
	out := transformStylesheet(t, xsl, serializeInput, nil, StylesheetOptions{})
	if out != "billing=BILL-001~\r\n" {
		t.Errorf("text output method: got %q, want %q", out, "billing=BILL-001~\r\n")
	}
}
