package xslt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/freemed/gokogiri/xml"
)

// Regression tests for the two properties that make the REMITT output
// reproducible, both of which were broken:
//
//  1. Global variable/parameter evaluation order must be determined by the
//     stylesheet, not by Go's randomised map iteration. Globals used to be
//     evaluated by ranging over Stylesheet.Variables, so a global that
//     referenced another global could be evaluated before its dependency
//     existed; the unresolved reference resolved to an empty node-set, and the
//     resulting wrong value (0, or NaN from a numeric addition) was frozen
//     because the retry passes only re-ran variables whose Value was still nil.
//     The same binary, input and stylesheet therefore produced 15286, 15287,
//     15288 and 15289 bytes for 4010_837p.xsl on consecutive runs.
//
//  2. The X12 HL hierarchical level of the 2000C (dependent) loop must be the
//     dependent level, i.e. `concat($patienthl, 'x', $practice)` where
//     $patienthl = $patienthlrel + $patientoffset and
//     $patientoffset = count($insuredall) + $insuredoffset. With (1) fixed,
//     count($insuredall) is 1, $insuredoffset is 1, so $patientoffset is 2,
//     $patienthlrel is 1 (position() inside sequence-location) and the level is
//     3 — matching xsltproc.

// globalOrderInput has one insured (subject) and one patient (dependent), the
// shape the 2000B/2000C loops of the 837p stylesheets walk.
const globalOrderInput = `<remitt>
<practice id="1"><name>PRACTICE</name></practice>
<insured id="1"><lastname>DOE</lastname><relationship>18</relationship></insured>
<patient id="1"><lastname>DOE</lastname></patient>
</remitt>`

// processString parses an XSLT stylesheet from a string and processes the given
// input document, returning the trimmed output. It mirrors the production call
// shape: ParseStylesheet followed by Stylesheet.Process.
func processString(t *testing.T, xslBody, input string, params map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	xslFile := filepath.Join(dir, "test.xsl")
	inFile := filepath.Join(dir, "test.xml")
	if err := os.WriteFile(xslFile, []byte(xslPrefix+xslBody+"</xsl:stylesheet>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inFile, []byte(input), 0o644); err != nil {
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
	return strings.TrimSpace(out)
}

// globalDependencyStylesheet declares its globals in the worst possible order:
// every dependent variable appears before the variable it depends on. The old
// map-order evaluation produced 0/NaN for these on some runs and the correct
// value on others.
const globalDependencyStylesheet = `<xsl:variable name="hllevel" select="$hlrel + $hloffset"/>` +
	`<xsl:variable name="hloffset" select="count($insuredall)+$insuredoffset"/>` +
	`<xsl:variable name="insuredoffset" select="1"/>` +
	`<xsl:variable name="hlrel" select="1"/>` +
	`<xsl:variable name="insuredall" select="//insured"/>` +
	`<xsl:template match="/remitt"><out>` +
	`<insureds><xsl:value-of select="count($insuredall)"/></insureds>` +
	`<offset><xsl:value-of select="$hloffset"/></offset>` +
	`<level><xsl:value-of select="$hllevel"/></level>` +
	`</out></xsl:template>`

// TestGlobalVariableDependencyOrder asserts that a global variable referencing
// another global always sees the referenced value, whatever order the two are
// declared in.
func TestGlobalVariableDependencyOrder(t *testing.T) {
	want := `<out><insureds>1</insureds><offset>2</offset><level>3</level></out>`
	for run := 0; run < 20; run++ {
		got := processString(t, globalDependencyStylesheet, globalOrderInput, nil)
		if got != want {
			t.Fatalf("run %d: output mismatch\n got: %s\nwant: %s", run, got, want)
		}
	}
}

// TestGlobalVariableEvaluationDeterministic processes the same stylesheet
// repeatedly — both from a fresh parse and from a reused Stylesheet — and
// requires byte-identical output every time. Deterministic output is what makes
// byte-parity with xsltproc possible at all.
func TestGlobalVariableEvaluationDeterministic(t *testing.T) {
	want := `<out><insureds>1</insureds><offset>2</offset><level>3</level></out>`

	// Fresh parse per run (how production calls the engine).
	first := ""
	for run := 0; run < 25; run++ {
		got := processString(t, globalDependencyStylesheet, globalOrderInput, nil)
		if run == 0 {
			first = got
		}
		if got != first {
			t.Fatalf("fresh-parse run %d differs:\n run 0: %q\n run %d: %q", run, first, run, got)
		}
	}
	if first != want {
		t.Fatalf("output mismatch\n got: %s\nwant: %s", first, want)
	}

	// One parse, many Process calls: the stylesheet must not become
	// order-dependent (or accumulate state) across runs.
	dir := t.TempDir()
	xslFile := filepath.Join(dir, "reuse.xsl")
	inFile := filepath.Join(dir, "reuse.xml")
	if err := os.WriteFile(xslFile, []byte(xslPrefix+globalDependencyStylesheet+"</xsl:stylesheet>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inFile, []byte(globalOrderInput), 0o644); err != nil {
		t.Fatal(err)
	}
	style, err := xml.ReadFile(xslFile, xml.StrictParseOption)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := xml.ReadFile(inFile, xml.StrictParseOption)
	if err != nil {
		t.Fatal(err)
	}
	ss, err := ParseStylesheet(style, xslFile)
	if err != nil {
		t.Fatal(err)
	}
	reusedFirst := ""
	for run := 0; run < 25; run++ {
		out, err := ss.Process(doc, StylesheetOptions{})
		if err != nil {
			t.Fatalf("process %d: %v", run, err)
		}
		got := strings.TrimSpace(out)
		if run == 0 {
			reusedFirst = got
		}
		if got != reusedFirst {
			t.Fatalf("reused-stylesheet run %d differs:\n run 0: %q\n run %d: %q", run, reusedFirst, run, got)
		}
	}
	if reusedFirst != want {
		t.Fatalf("reused output mismatch\n got: %s\nwant: %s", reusedFirst, want)
	}
}

// TestGlobalVariableOrderingHelpers pins the ordering primitives the fix is
// built on: declaration order is retained, and dependencies are discovered
// statically — including through xsl:call-template, since a global whose body
// calls a template depends on every global that template reads.
func TestGlobalVariableOrderingHelpers(t *testing.T) {
	body := `<xsl:variable name="d" select="$c + 1"/>` +
		`<xsl:variable name="c" select="$b + 1"/>` +
		`<xsl:variable name="b" select="$a"/>` +
		`<xsl:variable name="a" select="//practice"/>` +
		`<xsl:variable name="rtf"><xsl:call-template name="t"/></xsl:variable>` +
		`<xsl:template name="t"><xsl:value-of select="$b"/></xsl:template>` +
		`<xsl:template match="/remitt"><out/></xsl:template>`

	dir := t.TempDir()
	xslFile := filepath.Join(dir, "helpers.xsl")
	if err := os.WriteFile(xslFile, []byte(xslPrefix+body+"</xsl:stylesheet>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	style, err := xml.ReadFile(xslFile, xml.StrictParseOption)
	if err != nil {
		t.Fatal(err)
	}
	ss, err := ParseStylesheet(style, xslFile)
	if err != nil {
		t.Fatal(err)
	}

	gotOrder := strings.Join(ss.globalVariableNames(), ",")
	wantOrder := "d,c,b,a,rtf"
	if gotOrder != wantOrder {
		t.Errorf("declaration order not preserved\n got: %s\nwant: %s", gotOrder, wantOrder)
	}

	deps := map[string][]string{
		"d":   {"c"},
		"c":   {"b"},
		"b":   {"a"},
		"a":   {},
		"rtf": {"b"},
	}
	for name, want := range deps {
		got := strings.Join(ss.globalVariableDependencies(name), ",")
		if got != strings.Join(want, ",") {
			t.Errorf("dependencies of %s\n got: %s\nwant: %s", name, got, strings.Join(want, ","))
		}
	}

	// Repeated evaluation of the same helpers must give the same answer
	// (namespace and key lookups that used to range over maps).
	for run := 0; run < 10; run++ {
		if o := strings.Join(ss.globalVariableNames(), ","); o != wantOrder {
			t.Fatalf("run %d: declaration order changed: %s", run, o)
		}
	}
}

// TestHlHierarchicalLevelExpression reproduces the HL segment computation of
// resources/xsl/4010_837p.xsl: the 2000B (subscriber) HL and the 2000C
// (dependent) HL, whose level comes from $patienthlrel + $patientoffset where
// $patientoffset is count($insuredall) + $insuredoffset. xsltproc emits 2x1 for
// the subscriber and 3x1 for the dependent; the in-process engine used to emit
// 2x1/NaNx1 depending on map iteration order.
func TestHlHierarchicalLevelExpression(t *testing.T) {
	body := `<xsl:variable name="lowercase" select="'abcdefghijklmnopqrstuvwxyz'"/>` +
		`<xsl:variable name="uppercase" select="'ABCDEFGHIJKLMNOPQRSTUVWXYZ'"/>` +
		`<xsl:variable name="insuredall" select="//insured"/>` +
		`<xsl:variable name="patientall" select="//patient"/>` +
		`<xsl:variable name="insuredoffset" select="1"/>` +
		`<xsl:variable name="patientoffset" select="count($insuredall)+$insuredoffset"/>` +
		`<xsl:template match="/remitt">` +
		`<xsl:variable name="insured" select="'1'"/>` +
		`<xsl:variable name="patient" select="'1'"/>` +
		`<xsl:variable name="practice" select="'1'"/>` +
		`<xsl:variable name="insuredobj" select="//insured[@id=$insured]"/>` +
		`<xsl:variable name="insuredhlrel"><xsl:call-template name="sequence-location"><xsl:with-param name="id" select="$insured"/><xsl:with-param name="nodes" select="$insuredall"/></xsl:call-template></xsl:variable>` +
		`<xsl:variable name="insuredhl" select="$insuredhlrel + $insuredoffset"/>` +
		`<xsl:variable name="patienthlrel"><xsl:call-template name="sequence-location"><xsl:with-param name="id" select="$patient"/><xsl:with-param name="nodes" select="$patientall"/></xsl:call-template></xsl:variable>` +
		`<xsl:variable name="patienthl" select="$patienthlrel + $patientoffset"/>` +
		`<out>` +
		`<subscriber><hl><xsl:value-of select="concat($insuredhl, 'x', $practice)"/></hl><content>1</content></subscriber>` +
		`<xsl:if test="not($insuredobj/relationship = 'S')">` +
		`<dependent><hl><xsl:value-of select="concat($patienthl, 'x', $practice)"/></hl>` +
		`<content><xsl:value-of select="concat($insuredhl, 'x', $practice)"/></content></dependent>` +
		`</xsl:if>` +
		`</out></xsl:template>` +
		`<xsl:template name="sequence-location">` +
		`<xsl:param name="id"/><xsl:param name="nodes"/>` +
		`<xsl:for-each select="$nodes"><xsl:if test="@id = $id"><xsl:value-of select="position()"/></xsl:if></xsl:for-each>` +
		`</xsl:template>`

	// relationship 18 = dependent, so the 2000C loop runs and its HL level is 3.
	want := `<out><subscriber><hl>2x1</hl><content>1</content></subscriber>` +
		`<dependent><hl>3x1</hl><content>2x1</content></dependent></out>`
	for run := 0; run < 20; run++ {
		got := processString(t, body, globalOrderInput, nil)
		if got != want {
			t.Fatalf("run %d: HL level mismatch\n got: %s\nwant: %s", run, got, want)
		}
	}
}

// TestNamespaceDeclarationOrderDeterministic guards the other output path that
// used to range over a map: the namespace declarations copied onto the first
// output element come from Stylesheet.NamespaceMapping, and must be emitted in
// the order the stylesheet declared them, identically on every run.
func TestNamespaceDeclarationOrderDeterministic(t *testing.T) {
	dir := t.TempDir()
	xslFile := filepath.Join(dir, "ns.xsl")
	inFile := filepath.Join(dir, "ns.xml")
	xsl := `<?xml version="1.0"?>
<xsl:stylesheet version="1.0"
    xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
    xmlns:zeta="urn:example:zeta"
    xmlns:alpha="urn:example:alpha"
    xmlns:mid="urn:example:mid">
<xsl:template match="/remitt"><out><v><xsl:value-of select="count(//practice)"/></v></out></xsl:template>
</xsl:stylesheet>`
	if err := os.WriteFile(xslFile, []byte(xsl), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inFile, []byte(variableTestInput), 0o644); err != nil {
		t.Fatal(err)
	}
	style, err := xml.ReadFile(xslFile, xml.StrictParseOption)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseStylesheet(style, xslFile); err != nil {
		t.Fatal(err)
	}

	first := ""
	want := `<out xmlns:zeta="urn:example:zeta" xmlns:alpha="urn:example:alpha" xmlns:mid="urn:example:mid"><v>2</v></out>`
	for run := 0; run < 20; run++ {
		// Re-read per run: a fresh parse must produce the same order as any
		// other fresh parse, which is exactly what the map iteration broke.
		style, err := xml.ReadFile(xslFile, xml.StrictParseOption)
		if err != nil {
			t.Fatal(err)
		}
		ss, err := ParseStylesheet(style, xslFile)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := xml.ReadFile(inFile, xml.StrictParseOption)
		if err != nil {
			t.Fatal(err)
		}
		out, err := ss.Process(doc, StylesheetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		got := strings.TrimSpace(out)
		if run == 0 {
			first = got
		}
		if got != first {
			t.Fatalf("run %d differs:\n run 0: %q\n run %d: %q", run, first, run, got)
		}
	}
	// Declaration order, which is also what libxslt emits here:
	// xsltproc ns2.xsl nsin.xml == <out xmlns:zeta=... xmlns:alpha=... xmlns:mid=...>
	got := strings.TrimPrefix(first, `<?xml version="1.0"?>`+"\n")
	if got != want {
		t.Fatalf("namespace declaration order mismatch\n got: %s\nwant: %s", got, want)
	}
}
