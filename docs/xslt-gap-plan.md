# Ratago XSLT Gap Implementation Plan

Plan for implementing missing/incomplete XSLT 1.0 features found in `xsltproc`
but absent from ratago. Ordered by ascending effort; hardest phases last.

---

## Phase 1: Quality Edge Cases & Stubs (low effort, 2-3 days)

Small fixes that re-enable disabled tests or eliminate runtime "TODO" prints.

### 1.1  `element-available()` always returns false

**File:** `xslt/functions.go:176-178`
**Fix:** Return true for standard XSLT elements (apply-templates, call-template,
element, attribute, text, value-of, for-each, if, choose, copy, copy-of, number,
message, etc.).

### 1.2  `generate-id()` for context node returns hardcoded "N"

**File:** `xslt/functions.go:130`
**Fix:** When no argument, call `generate-id()` on the context (current) node.

### 1.3  `function-available()` doesn't resolve namespace

**File:** `xslt/functions.go:170-172`
**Fix:** Accept QName, resolve namespace prefix via `LookupNamespace`, call
`IsFunctionRegistered(ns, localname)`.

### 1.4  `ResolveQName` ignores default namespace

**File:** `xslt/context.go:195-197`
**Fix:** When no prefix, look up the default namespace from the in-scope
namespaces on the context node or stylesheet.

### 1.5  Duplicate named template detection

**File:** `xslt/stylesheet.go:653`
**Fix:** Before inserting into `NamedTemplates`, check if name already exists
and return an error (spec requires error on duplicate named templates).

### 1.6  `xsl:message terminate=yes` panics instead of graceful error

**File:** `xslt/instruction.go:408-409`
**Fix:** Instead of `panic(val)`, return an error from `Apply()` that propagates
up through `Process()`. May require changing `Apply` signature or using a
sentinel error checked by caller.

### 1.7  `xsl:apply-imports` stub

**File:** `xslt/instruction.go:413-414`
**Fix:** In `LookupTemplate`, after the import-precedence walk finds a match,
store the "next best" template. Then `xsl:apply-imports` invokes the next
template down the import chain. Requires:
- Add a `NextMatch` pointer or method on `ExecutionContext`
- Modify `LookupTemplate` to track which import level matched
- In `XsltInstruction.Apply` case `"apply-imports"`: call the next match

### 1.8  `xsl:decimal-format` storage

**File:** `xslt/stylesheet.go:296-299`
**Fix:** Define a `DecimalFormat` struct (name, decimal-separator, grouping-separator,
infinity, minus-sign, NaN, percent, per-mille, zero-digit, digit, pattern-separator).
Add `DecimalFormats map[string]*DecimalFormat` to `Stylesheet`. Parse children
of `xsl:decimal-format` and store. Default format "no name" gets spec defaults.

### 1.9  Missing attribute-node default rule

**File:** `xslt/stylesheet.go:573-585`
**Fix:** Add case for `XML_ATTRIBUTE_NODE` in `processDefaultRule` — should
create a text node with the attribute's value.

### 1.10  Add error generation for spec violations

- `xsl:attribute` with non-text children (`instruction.go:48`)
- `xsl:processing-instruction` containing `?>` in value (`instruction.go:199`)

**Fix:** In `evalChildrenAsText`, check child node types and return error.
In PI Apply, check for `?>` in val and return error.

**Re-enables tests:** bug-56 (possibly), improves correctness

---

## Phase 2: Missing `xsl:namespace` Instruction (1 day)

### 2.1  Implement `xsl:namespace`

**File:** `xslt/instruction.go` — add case `"namespace"` to Apply switch

**Logic:**
1. Read `name` attribute (AVT allowed).
2. Evaluate children as text (like `xsl:attribute`).
3. Call `context.OutputNode.DeclareNamespace(name, value)`.
4. Error if name is `xmlns` or starts with `xmlns:`.

---

## Phase 3: Namespace Handling Fixes (3-5 days)

This single category re-enables the most disabled tests (~10).

### 3.1  `xsl:copy-of` must copy namespace nodes

**File:** `xslt/instruction.go:498-571` (`copyToOutput`)
**Fix:** When `recursive=true` and node is an element:
- Before copying children, collect all namespace declarations in scope
- Call `r.DeclareNamespace(prefix, uri)` for each
- Avoid duplicating NS already declared on `r`

**Re-enables:** bug-38, bug-54, bug-87, bug-104, bug-122, bug-124

### 3.2  Namespace declaration ordering closer to libxslt

**File:** `xslt/instruction.go` in LRE and xsl:element
**Fix:** Several tests fail only because namespace declaration *order* differs
from libxslt output. Consider sorting declarations or mimicking libxslt's
pre-order walk. Alternatively, accept looser matching in tests.

**Re-enables:** bug-54, bug-71, bug-99

### 3.3  `xsl:element` should reuse in-scope namespaces

**File:** `xslt/instruction.go:158-170`
**Fix:** Before creating a new namespace declaration, check if the namespace
is already in-scope on the parent element. If so, don't redeclare it.

**Re-enables:** bug-179

### 3.4  Avoid namespace declarations for `xml:` namespace

**File:** `xslt/context.go:327-341` (`DeclareStylesheetNamespacesIfRoot`)
**Fix:** Skip `XML_NAMESPACE` when adding declarations to root.

**Re-enables:** bug-177

### 3.5  Literal result element default namespace scoping

**File:** `xslt/template.go:156-163` (`LiteralResultElement.Apply`)
**Fix:** When a child element has no namespace, it should inherit the default
namespace from its parent in-scope namespaces, not just the top-level mapping.

**Re-enables:** bug-150

---

## Phase 4: Import / Include Edge Cases (2-3 days)

### 4.1  `xsl:output` effective merge on import

**File:** `xslt/stylesheet.go:245-271`, `stylesheet.go:376-442`
**Fix:** When merging imported stylesheets, `xsl:output` settings from the
importing stylesheet should override those from imported ones (higher import
precedence wins). Currently imports are stored but their output settings
aren't consulted during `constructOutput`.

**Approach:** At the end of `ParseStylesheet`, walk imports and merge output
settings with the rule that the importer's settings win.

**Re-enables:** bug-93

### 4.2  `xsl:attribute-set` import combine

**File:** `xslt/stylesheet.go:550-554`, `stylesheet.go:728-741`
**Fix:** When an importing stylesheet defines an attribute set with the same
name as one in an imported stylesheet, the importing set's attributes should
be merged in (not replaced). `LookupAttributeSet` already chains through
imports but `RegisterAttributeSet` should combine rather than overwrite.

**Re-enables:** bug-102, bug-131

### 4.3  `document()` base URI in imported stylesheets

**File:** `xslt/context.go:343-372` (`FetchInputDocument`)
**Fix:** When `document()` is called from a template in an imported stylesheet,
relative URIs should resolve against the imported stylesheet's location, not
the importing stylesheet's location. Track the "current stylesheet" URI in
the execution context.

**Re-enables:** bug-130

### 4.4  Template import precedence: delete vs override

**File:** `xslt/stylesheet.go:447-548` (`LookupTemplate`)
**Review:** Check that a template in importing stylesheet with same match
*completely replaces* imported templates (not just competing on priority).
Spec says higher import precedence wins.

**Re-enables:** bug-147

---

## Phase 5: XSLT Core Function Improvements (2-3 days)

### 5.1  `format-number()` overhaul

**File:** `xslt/functions.go:282-325`
**Fix:**
- Handle NaN → "NaN" string
- Handle Infinity → "Infinity" string
- Handle negative numbers → prepend minus sign
- Support grouping-separator from `xsl:decimal-format`
- Support scientific notation patterns
- Wire `xsl:decimal-format` settings (percent, per-mille, etc.)

**Re-enables:** bug-61, bug-75, bug-95

### 5.2  `xsl:number` QName support in count attribute

**File:** `xslt/instruction.go:441-443`, `xslt/number.go:255-263` (`findTarget`)
**Fix:** When count is a QName with prefix, resolve the namespace. The
`CompileMatch` in `findTarget` should match on namespace+localname, not
just localname.

### 5.3  `xsl:number` alpha index > 701

**File:** `xslt/number.go:60-70` (`toAlphaIndex`)
**Fix:** Replace or extend the current algorithm. For Excel-style column
numbering (a..z, aa..az, ba..bz, ...), use:
```go
func toAlphaIndex(n int) string {
    var result string
    for n > 0 {
        n--
        result = string(rune('a'+n%26)) + result
        n /= 26
    }
    return result
}
```

### 5.4  `xsl:number` format string edge cases

**File:** `xslt/number.go:153-155`
**Fix:** Handle format strings with zero numbers (trailing punctuation-only)
and with punctuation-only tokens.

---

## Phase 6: `xsl:key` Improvements (2 days)

### 6.1  Multiple keys with same name

**File:** `xslt/stylesheet.go:176-183`, `xslt/functions.go:42-69`
**Fix:** Allow multiple `xsl:key` elements with the same name. When populating
keys, nodes matching any same-named key's match pattern should be indexed
under that name. `XSLTKey` needs to union results from all keys with that name.

**Re-enables:** bug-128

### 6.2  Complex match patterns on `xsl:key`

**File:** `xslt/stylesheet.go:608-618` (`populateKeys`)
**Fix:** Currently matches keys using `CompileMatch`, which handles basic
patterns. The issue with `node()[self::sect]` is that the key's match
pattern context needs to be set correctly. Ensure the XPath context is
set to the document root when evaluating key match patterns.

**Re-enables:** bug-134, bug-135

### 6.3  Key interaction with `generate-id()`

**Re-enables:** bug-133 (likely automatically fixed by 6.1 or 1.2)

---

## Phase 7: Whitespace & Preserve-Space (1 day)

### 7.1  `xsl:strip-space` / `xsl:preserve-space` with QNames

**File:** `xslt/context.go:163-193` (`ShouldStrip`)
**Fix:** The `StripSpace` and `PreserveSpace` lists should be stored as
resolved (namespace, localname) pairs, like CDataElements. `ShouldStrip`
should resolve QNames when checking. Resolve conflicts per spec:
preserve-space beats strip-space for equal specificity.

### 7.2  Global `xml:space=preserve` not honored

**File:** `xslt/context.go:164-171`
**Fix:** Walk ancestors in `ShouldStrip` checking for `xml:space="preserve"`.

**Re-enables:** bug-82

---

## Phase 8: High-Effort Items (left for last)

These require deeper architectural changes or are naturally larger scope.

### 8.1  Non-UTF-8 encoding output

**File:** `xslt/stylesheet.go:414-416`
**Complexity:** High. Requires encoding-aware serialization pipeline.
Could use `golang.org/x/text/encoding` to transcode output bytes.
Alternatively, focus on ASCII entity-escaping for now.

**Re-enables:** bug-139, bug-159, bug-169, bug-175

### 8.2  Template priority edge cases

**File:** `xslt/match.go:622-669` (`DefaultPriority`)
**Complexity:** Medium. Need to correctly calculate priority for
`text()[2]`, `comment()[position()>1]`, and other predicate-bearing
node tests. Current logic drops to 0.5 default too aggressively.

**Re-enables:** bug-160, bug-181, bug-182

### 8.3  AVT parsing edge cases

**File:** `xslt/template.go:208-267` (`evalAVT`)
**Complexity:** Medium-High. Nested braces, escaped braces, and
interactions with XPath quoting need careful handling.

**Re-enables:** bug-126, bug-168

### 8.4  EXSLT extensions (date, func, strings, sets)

**Complexity:** High. Each module is substantial:
- `date:` functions (date-time, add, difference, etc.)
- `func:function` (user-defined XSLT functions — whole new subsystem)
- `strings:` (tokenize, replace, padding)
- `sets:` (intersection, difference, distinct, etc.)

**Re-enables:** bug-65, bug-100, bug-137, bug-174, bug-178, date_add

### 8.5  Variable evaluation ordering

**File:** `xslt/stylesheet.go:338-341`
**Complexity:** High. Global variables with circular or cross-references
need dependency analysis. Variables used in predicates that reference
other variables that haven't been evaluated yet cause failures.

**Re-enables:** bug-143, bug-165

### 8.6  XSLT 2.0 / XPath 2.0

**File:** `xpath2/` (early WIP)
**Complexity:** Enormous. `xpath2/lexer.go` and `xpath2/grammar.go` exist
but are incomplete. Full 2.0 is its own project.

---

## Summary

| Phase | Effort | Tests Re-enabled | Description |
|---|---|---|---|
| 1 | 2-3 days | 3-5 | Quality edge cases, stubs |
| 2 | 1 day | 0-1 | `xsl:namespace` instruction |
| 3 | 3-5 days | ~10 | Namespace handling fixes |
| 4 | 2-3 days | 4-6 | Import/include edge cases |
| 5 | 2-3 days | 3-5 | format-number, xsl:number fixes |
| 6 | 2 days | 3-4 | xsl:key improvements |
| 7 | 1 day | 1-2 | Whitespace/preserve-space |
| 8 | Open-ended | 15-20 | High-effort items |
| **Total** | **13-18 days (Phases 1-7)** | **25-33 tests** | Core gap closure |

---

## Implementation Strategy

1. Work phase by phase, completing all items in a phase before moving on.
2. After each phase, uncomment the corresponding `runGeneralXslTest` calls
   in `xslt/stylesheet_test.go` and run `go test ./xslt/`.
3. If a test still fails, investigate and fix before proceeding.
4. For Phases 1-4 (which share namespace-related code), prefer a single
   branch with incremental commits per item.
5. Phases 5-7 are independent enough to parallelize.
6. Phase 8 items should be tackled one-at-a-time as standalone efforts.
