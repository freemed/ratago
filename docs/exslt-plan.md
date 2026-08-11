# EXSLT 1.0 Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan module-by-module.

**Goal:** Implement all EXSLT 1.0 extension modules (math, sets, strings, dates-and-times, common, functions, dynamic, random) in ratago's native Go XSLT processor.

**Architecture:** Each EXSLT module is registered in `RegisterXsltFunctions()` using the same `map[string]xpath.XPathFunction` pattern already in place. Functions live in `xslt/functions.go` until file size warrants splitting into `xslt/exslt_math.go`, `xslt/exslt_sets.go`, etc. The `func:function` module requires a new `exslt_functions.go` with element-parsing support in `stylesheet.go`'s `parseChildren()`.

**Tech Stack:** Go 1.24, gokogiri CGo XML/XPath, existing `xslt` package conventions.

---

## Current EXSLT Coverage

| Module | Namespace | Implemented | Missing |
|--------|-----------|-------------|---------|
| Common | `http://exslt.org/common` | `node-set` (also `xmlsoft.org/XSLT/namespace` variant) | `object-type` |
| Math | `http://exslt.org/math` | `constant`, `sin`, `cos`, `abs` | `min`, `max`, `sqrt`, `power`, `tan`, `asin`, `acos`, `atan`, `atan2`, `exp`, `log`, `random`, `highest`, `lowest` |
| Sets | `http://exslt.org/sets` | *none* | `difference`, `distinct`, `has-same-node`, `intersection`, `leading`, `trailing` |
| Strings | `http://exslt.org/strings` | *none* | `concat`, `split`, `tokenize`, `replace`, `padding`, `align` |
| Dates | `http://exslt.org/dates-and-times` | *none* | `date-time`, `date`, `time`, `year`, `month-in-year`, `day-in-month`, `day-in-year`, `hour-in-day`, `minute-in-hour`, `second-in-minute`, `week-in-year`, `day-in-week`, `add`, `add-duration`, `duration`, `sum`, `seconds`, `difference` |
| Functions | `http://exslt.org/functions` | *none* | `func:function`, `func:result` (extension-element + function subsystem) |
| Dynamic | `http://exslt.org/dynamic` | *none* | `evaluate`, `closure` |
| Random | `http://exslt.org/random` | *none* | `random-sequence` |

**Tests re-enabled by this work:** bug-65 (node-set fix), bug-116 (set:distinct bug), bug-137 (func:function), bug-174 (func:result error), bug-178 (func:function), date_add (date:add)

---

## Module 1: EXSLT Math (3-4 hours)

**Effort:** Low. All functions are pure math operations on `float64`. No node-set handling required.

**File:** `xslt/functions.go` (add implementations + register in `RegisterXsltFunctions()`)

### Functions to implement

All take `xpath.VariableScope, []interface{}` and return `interface{}`. Each validates arg count, asserts `float64` on numeric args.

**1.1 `math:min`**
```
math:min(node-set) → number
```
Returns the minimum of the numeric values of all nodes in the node-set. If node-set is empty, returns NaN.

**1.2 `math:max`**
```
math:max(node-set) → number
```
Returns the maximum. Empty node-set → NaN.

**1.3 `math:sqrt`**
```
math:sqrt(number) → number
```
`math.Sqrt(val)`. Error on negative input.

**1.4 `math:power`**
```
math:power(number, number) → number
```
`math.Pow(base, exp)`.

**1.5 `math:tan`**
```
math:tan(number) → number
```
`math.Tan(val)`.

**1.6 `math:asin`**
```
math:asin(number) → number
```
`math.Asin(val)`. Clamp input to [-1,1] per spec.

**1.7 `math:acos`**
```
math:acos(number) → number
```
`math.Acos(val)`. Clamp input to [-1,1].

**1.8 `math:atan`**
```
math:atan(number) → number
```
`math.Atan(val)`.

**1.9 `math:atan2`**
```
math:atan2(number, number) → number
```
`math.Atan2(y, x)`.

**1.10 `math:exp`**
```
math:exp(number) → number
```
`math.Exp(val)`.

**1.11 `math:log`**
```
math:log(number) → number
```
`math.Log(val)`. Error on non-positive input.

**1.12 `math:random`**
```
math:random() → number
math:random(number) → number
```
If no arg: returns `rand.Float64()`. If arg is true: seeds with current time and returns 0-1 random. If arg is number: seeds with that number and returns a repeatable random.

Use `math/rand/v2` for seeding. If arg is a boolean `true`, reseed. If numeric, treat as seed.

**1.13 `math:highest`**
```
math:highest(node-set) → node-set
```
Returns the node(s) in the set with the highest numeric value. If multiple tie, all are returned.

**1.14 `math:lowest`**
```
math:lowest(node-set) → node-set
```
Same but lowest.

### Registration
Add all to `RegisterXsltFunctions()` under `"http://exslt.org/math"` namespace prefix.

### Verification
```bash
go build ./xslt/
go test ./xslt/ -run TestGeneral -count=1
```

No specific EXSLT math tests in the suite, so correctness is verified by build + no regressions.

---

## Module 2: EXSLT Sets (4-5 hours)

**Effort:** Medium. Requires node-set comparison, identity checking, and careful set semantics.

**File:** `xslt/functions.go` (or new `xslt/exslt_sets.go` if file gets large)

### Core helpers needed

```go
// nodesetFromArg extracts a []xml.Node from an interface{} arg
func nodesetFromArg(arg interface{}) []xml.Node

// nodeIdentity returns a unique pointer-based identity for a node
func nodeIdentity(node xml.Node) uintptr

// nodesetToXPath converts []xml.Node back to []unsafe.Pointer for XPath
func nodesetToXPath(nodes []xml.Node) []unsafe.Pointer
```

### Functions

**2.1 `set:difference`**
```
set:difference(node-set, node-set) → node-set
```
Returns nodes in first set that are NOT in second set. Use node identity (pointer) for comparison.

**2.2 `set:intersection`**
```
set:intersection(node-set, node-set) → node-set
```
Returns nodes present in BOTH sets. Same identity-based comparison.

**2.3 `set:distinct`**
```
set:distinct(node-set) → node-set
```
Returns nodes with unique string values. For each node in the set, compare its `String()` value; keep only the first occurrence with each value. This is the function used in the bug-116 test.

**2.4 `set:has-same-node`**
```
set:has-same-node(node-set, node-set) → boolean
```
Returns true if the two node-sets share at least one identical node. Uses pointer identity.

**2.5 `set:leading`**
```
set:leading(node-set, node-set) → node-set
```
Returns nodes in first set that come BEFORE the first node that is also in the second set. Document order comparison required. Uses `xml.Node.IsSameNode()` or compares node pointers since gokogiri nodes have a deterministic order.

**2.6 `set:trailing`**
```
set:trailing(node-set, node-set) → node-set
```
Returns nodes in first set that come AFTER the first node that is also in the second set.

### Registration
Add to `RegisterXsltFunctions()` under `"http://exslt.org/sets"`.

### Re-enables test
- **bug-116**: Uncomment `runGeneralXslTest(t, "bug-116")` — it's already running (line 250), but currently passes only because the XPath selects an empty node-set. After implementation, verify it still passes.

### Verification
```bash
go build ./xslt/
go test ./xslt/ -run TestGeneral -count=1
```

---

## Module 3: EXSLT Strings (4-5 hours)

**Effort:** Medium. String manipulation, pattern matching, padding, and alignment. Pure string operations on values, not node identities.

**File:** `xslt/functions.go` (or new `xslt/exslt_strings.go`)

### Functions

**3.1 `str:concat`**
```
str:concat(node-set) → string
str:concat(node-set, string) → string
```
Concatenates string values of all nodes in the node-set. If a second arg is given, it's used as a separator between values.

**3.2 `str:split`**
```
str:split(string) → node-set
str:split(string, string) → node-set
```
Splits the string by whitespace (first form) or by the given pattern (second form). Returns a node-set of `<token>` elements in the `http://exslt.org/strings` namespace.

Implementation: Create faux elements in the output document. Standard pattern from `exslt:node-set`:
```go
fauxroot := c.Output.CreateElementNode("token")
fauxroot.SetNamespace("http://exslt.org/strings", "str")
// add child text nodes for each token...
```

**3.3 `str:tokenize`**
```
str:tokenize(string) → node-set
str:tokenize(string, string) → node-set
```
Same as `str:split`. The EXSLT spec says these are equivalent; `split` is the older name, `tokenize` the newer.

**3.4 `str:replace`**
```
str:replace(string, object, object) → string
```
Replace occurrences of the search string with the replacement string. The "object" args can be strings or node-sets (converted to string). Use `strings.ReplaceAll`.

**3.5 `str:padding`**
```
str:padding(number) → string
str:padding(number, string) → string
```
Creates a padding string of the given length. If a second arg is given, repeats that character; otherwise uses space (U+0020). The second arg is a string; if longer than 1 char, use the first character only.

**3.6 `str:align`**
```
str:align(string, string) → string
str:align(string, string, string) → string
```
Aligns a string within a given padding string. The second arg is the alignment pattern (like "left", "right", "center"). The third arg is the padding string (defaults to space).

This is the most complex strings function. Implementation:
- Parse the alignment: "left" → left-justify, "right" → right-justify, "center" → center.
- Pad/crop the string to fit the padding length.
- Alternative interpretation: the second arg is a template string like "______" and the first arg is placed inside it.

Standard EXSLT `str:align(string, pattern)` where pattern is like "right" or a literal padding string. Check the spec: `str:align(target, pattern, padding)` where pattern = "left"/"right"/"center" and padding is the character to pad with.

### Registration
Add to `RegisterXsltFunctions()` under `"http://exslt.org/strings"`.

### Verification
```bash
go build ./xslt/
go test ./xslt/ -run TestGeneral -count=1
```

---

## Module 4: EXSLT Dates and Times (6-8 hours)

**Effort:** High. Date parsing, arithmetic, duration handling, and ISO 8601 formatting. This is the largest single EXSLT module.

**File:** `xslt/exslt_date.go` (new file; too large for functions.go)

### Core date types

```go
type EXSLTDateTime struct {
    Year, Month, Day, Hour, Minute, Second int
    HasTime bool // true if time component is meaningful
}

type EXSLTDuration struct {
    Negative bool
    Years, Months, Days int
    Hours, Minutes, Seconds int
    FractionalSeconds float64
}
```

### Internal helpers

```go
// parseDateTime parses ISO 8601 / EXSLT date-time strings
func parseDateTime(s string) (EXSLTDateTime, error)

// formatDateTime formats according to ISO 8601
func (dt EXSLTDateTime) formatDate() string
func (dt EXSLTDateTime) formatDateTime() string
func (dt EXSLTDateTime) formatTime() string

// parseDuration parses ISO 8601 duration strings (P[nY][nM][nD][T[nH][nM][nS]])
func parseDuration(s string) (EXSLTDuration, error)

// addDuration adds a duration to a date-time
func addDuration(dt EXSLTDateTime, dur EXSLTDuration) EXSLTDateTime

// daysInMonth returns days in a given month/year
func daysInMonth(year, month int) int

// daysBeforeMonth returns cumulative days before a given month in a year
func daysBeforeMonth(year, month int) int
```

### Extractor functions (4.1-4.12)

These take a date-time string and return the appropriate component:

**4.1 `date:date-time`** — returns current date-time as ISO 8601 string. Uses `time.Now().UTC()`.

**4.2 `date:date`** — takes a date-time string and returns `YYYY-MM-DD` portion.

**4.3 `date:time`** — takes a date-time string and returns `HH:MM:SS` portion.

**4.4 `date:year`** — returns year as number.

**4.5 `date:month-in-year`** — returns month (1-12).

**4.6 `date:day-in-month`** — returns day (1-31).

**4.7 `date:day-in-year`** — returns day of year (1-366).

**4.8 `date:hour-in-day`** — returns hour (0-23).

**4.9 `date:minute-in-hour`** — returns minute (0-59).

**4.10 `date:second-in-minute`** — returns second (0-59).

**4.11 `date:week-in-year`** — returns ISO week number (1-53).

**4.12 `date:day-in-week`** — returns day of week (1=Monday through 7=Sunday per ISO 8601).

### Arithmetic functions

**4.13 `date:add`**
```
date:add(string, string) → string
```
Adds an ISO 8601 duration to a date-time. Returns the resulting date-time. Example: `date:add('2001-01', 'P3D')` → `'2001-01-04'`.

**4.14 `date:add-duration`**
```
date:add-duration(string, string) → string
```
Adds two durations together. Returns the sum duration.

**4.15 `date:duration`**
```
date:duration(number) → string
date:duration(string) → string
```
If number: returns duration of that many seconds as ISO 8601 duration. If string: normalizes a duration string to canonical form.

**4.16 `date:sum`**
```
date:sum(node-set) → string
```
Sums the durations in a node-set. Each node's string value must be an ISO 8601 duration.

**4.17 `date:seconds`**
```
date:seconds(string) → number
date:seconds(node-set) → number
```
If string: converts a duration to total seconds. If node-set: converts each node value to seconds and sums them.

**4.18 `date:difference`**
```
date:difference(string, string) → string
```
Computes the duration between two date-times. Returns an ISO 8601 duration.

### Date parsing note
EXSLT date parsing is lenient. Standard format: `YYYY-MM-DDThh:mm:ss` with optional fractional seconds and timezone. Also accept `YYYY-MM-DD` (date only). The `date:date()` accessor extracts the date portion.

For implementation, use Go's `time.Parse()` with multiple format strings as fallbacks:
```
"2006-01-02T15:04:05Z07:00"
"2006-01-02T15:04:05"
"2006-01-02"
"2006-01"
```

### Registration
Add to `RegisterXsltFunctions()` under `"http://exslt.org/dates-and-times"`.

### Re-enables test
- **date_add**: Add `runGeneralXslTest(t, "date_add")` to TestGeneral (after the bug-182 line) and uncomment.

### Verification
```bash
go build ./xslt/
go test ./xslt/ -run TestGeneral -count=1
```

---

## Module 5: EXSLT Common (1 hour)

**Effort:** Low. Only one missing function (`object-type`).

**File:** `xslt/functions.go`

### 5.1 `exslt:object-type`

```
exslt:object-type(object) → string
```
Returns a string describing the type of the object:
- "string" for string values
- "number" for numeric values
- "boolean" for boolean values
- "node-set" for node sets (`[]unsafe.Pointer`)
- "RTF" for result tree fragments (same as node-set handler output)

Implementation:
```go
func EXSLTobjectType(context xpath.VariableScope, args []interface{}) interface{} {
    if len(args) < 1 { return "string" }
    switch args[0].(type) {
    case string:      return "string"
    case float64:     return "number"
    case bool:        return "boolean"
    case []unsafe.Pointer: return "node-set"
    default:          return "string"
    }
}
```

### Registration
Add `style.Functions["{http://exslt.org/common}object-type"] = EXSLTobjectType`.

### Verification
```bash
go build ./xslt/
go test ./xslt/ -run TestGeneral -count=1
```

---

## Module 6: EXSLT Dynamic (2-3 hours)

**Effort:** Medium. XPath evaluation at runtime via string expressions.

**File:** `xslt/functions.go`

### 6.1 `dyn:evaluate`

```
dyn:evaluate(string) → object
dyn:evaluate(string, node-set) → object
```
Evaluates an XPath expression string at runtime. The optional second arg provides the context node. Uses `context.EvalXPath()`.

Implementation:
```go
func EXSLTdynEvaluate(context xpath.VariableScope, args []interface{}) interface{} {
    if len(args) < 1 { return nil }
    c := context.(*ExecutionContext)
    xpathStr := argValToString(args[0])
    evalNode := c.Current
    if len(args) >= 2 {
        // use provided node-set as context
        switch v := args[1].(type) {
        case []unsafe.Pointer:
            if len(v) > 0 {
                evalNode = xml.NewNode(v[0], nil)
            }
        }
    }
    result, err := c.EvalXPath(evalNode, xpathStr)
    if err != nil { return nil }
    return result
}
```

### 6.2 `dyn:closure` (optional/lower priority)

Creates a closure over an expression and a set of variable bindings. This is complex and rarely used. Defer to a follow-up if no tests exercise it.

### Registration
Add to `RegisterXsltFunctions()` under `"http://exslt.org/dynamic"`.

### Verification
```bash
go build ./xslt/
go test ./xslt/ -run TestGeneral -count=1
```

---

## Module 7: EXSLT Random (30 min)

**Effort:** Low. Single function.

**File:** `xslt/functions.go`

### 7.1 `random:random-sequence`

```
random:random-sequence() → number
random:random-sequence(number) → number
random:random-sequence(number, number) → node-set
```
If no arg: returns a single random number 0 <= n < 1. If one arg (number): returns that many random numbers as a node-set of `<random:random-sequence>` elements. If two args (seed, count): seeds the generator and returns count random numbers.

```go
func EXSLTrandomSequence(context xpath.VariableScope, args []interface{}) interface{} {
    c := context.(*ExecutionContext)
    switch len(args) {
    case 0:
        return rand.Float64()
    case 1:
        count := int(args[0].(float64))
        return generateSequence(c, count, 0)
    case 2:
        seed := int64(args[0].(float64))
        count := int(args[1].(float64))
        return generateSequence(c, count, seed)
    }
    return nil
}
```

### Registration
Add to `RegisterXsltFunctions()` under `"http://exslt.org/random"`.

---

## Module 8: EXSLT Functions (`func:function` / `func:result`) (8-12 hours)

**Effort:** Very High. This is an extension-element subsystem, not just new XPath functions. It requires:
1. Parsing `func:function` elements during stylesheet compilation
2. Registering user-defined functions in the function table
3. Handling `func:result` elements during function execution
4. `function-available()` must reflect these

**Files:** New `xslt/exslt_functions.go`, modify `xslt/stylesheet.go` (`parseChildren`), modify `xslt/functions.go` (registration)

### Architecture

#### 8.1 `exslt_functions.go` — User-Defined Function Type

```go
// UserFunction represents a func:function definition
type UserFunction struct {
    Name string
    NS   string // resolved namespace URI
    Args []string // parameter names
    Body xml.Node // the func:function element node (to execute as template)
    Style *Stylesheet // owning stylesheet
}

// Apply executes the user-defined function
func (uf *UserFunction) Apply(context xpath.VariableScope, args []interface{}) interface{} {
    c := context.(*ExecutionContext)
    // Push a new local scope
    c.PushStack()
    defer c.PopStack()
    
    // Bind arguments to parameter names
    for i, name := range uf.Args {
        if i < len(args) {
            v := &Variable{Name: name, Value: args[i]}
            c.DeclareLocalVariable(name, "", v)
        }
    }
    
    // Create an output fragment (like a result tree fragment)
    output := c.Output.CreateDocument()
    oldOutput := c.OutputNode
    oldOutputDoc := c.Output
    c.Output = output
    c.OutputNode = output
    
    // Execute the body children (templates/instructions)
    // BUT trap func:result to return early
    defer func() {
        c.Output = oldOutputDoc
        c.OutputNode = oldOutput
    }()
    
    // Walk children of uf.Body
    for child := uf.Body.FirstChild(); child != nil; child = child.NextSibling() {
        if IsBlank(child) { continue }
        // If we encounter func:result, return its value
        if child.Namespace() == "http://exslt.org/functions" && child.Name() == "result" {
            return evaluateFuncResult(child, c)
        }
        // Otherwise evaluate as regular content
        // (This is the tricky part — we need to execute XSLT instructions as content)
    }
    return nil
}
```

#### 8.2 Parsing `func:function` in `stylesheet.go`

In `parseChildren()`, add a case for `func:function` elements (after the existing XSLT element checks):

```go
if cur.Namespace() == "http://exslt.org/functions" && cur.Name() == "function" {
    name := cur.Attr("name")  // qualified name like "my:foo"
    // Resolve QName to NS + localname
    ns, local := style.ResolveQNameEx(cur, name)
    // Collect param names
    var argNames []string
    for c := cur.FirstChild(); c != nil; c = c.NextSibling() {
        if IsBlank(c) { continue }
        if IsXsltName(c, "param") {
            argNames = append(argNames, c.Attr("name"))
        }
    }
    uf := &UserFunction{Name: local, NS: ns, Args: argNames, Body: cur, Style: style}
    qname := fmt.Sprintf("{%s}%s", ns, local)
    style.Functions[qname] = uf.Apply
    continue
}
```

#### 8.3 `func:result` handling

The `func:result` element:
- Has an optional `select` attribute (XPath expression)
- Can have children that are evaluated as a result tree fragment
- Must be the last thing executed in a function (further siblings should cause an error per spec: bug-174)

```go
func evaluateFuncResult(node xml.Node, c *ExecutionContext) interface{} {
    selectAttr := node.Attr("select")
    if selectAttr != "" {
        return c.EvalXPath(c.Current, selectAttr)
    }
    // Evaluate children as a result tree fragment
    // This is similar to xsl:variable with content
    // Return the fragment as a node-set
}
```

#### 8.4 `exsltFuncResultElem` error (bug-174)

Per the EXSLT spec, `func:result` must be the last child; siblings after it (except `xsl:fallback`) are an error. The test bug-174 expects no output when this error occurs. Implement a check: after finding `func:result`, scan remaining siblings and if any non-whitespace, non-fallback elements exist, produce an error and return empty.

#### 8.5 Registration

In `RegisterXsltFunctions()`, register the `func:function` factory:
```go
// Note: actual functions are registered dynamically in parseChildren
// We don't register a static handler here; the stylesheet parser adds them.
```

### Re-enables tests
- **bug-137**: `runGeneralXslTest(t, "bug-137")` — tests func:function with key()
- **bug-174**: `runGeneralXslTest(t, "bug-174")` — tests func:result sibling error
- **bug-178**: `runGeneralXslTest(t, "bug-178")` — tests func:function with text child

### Verification
```bash
go build ./xslt/
go test ./xslt/ -run TestGeneral -count=1
```

---

## Implementation Order (Dependency Chain)

```
Module 1 (Math)     ── independent ──┐
Module 2 (Sets)     ── independent ──┤
Module 3 (Strings)  ── independent ──┼── Modules 1-7 can be parallelized
Module 4 (Dates)    ── independent ──┤
Module 5 (Common)   ── independent ──┤
Module 6 (Dynamic)  ── independent ──┤
Module 7 (Random)   ── independent ──┘
Module 8 (func)     ── last: touches parseChildren core path, highest risk of regressions
```

## Summary

| Module | Effort | Complexity | Tests Re-enabled |
|--------|--------|-----------|------------------|
| 1. Math | 3-4 hrs | Low | 0 (no test suite entries) |
| 2. Sets | 4-5 hrs | Medium | bug-116 (verify still passes) |
| 3. Strings | 4-5 hrs | Medium | 0 (no test suite entries) |
| 4. Dates | 6-8 hrs | High | date_add |
| 5. Common | 1 hr | Low | 0 |
| 6. Dynamic | 2-3 hrs | Medium | 0 |
| 7. Random | 30 min | Low | 0 |
| 8. Functions | 8-12 hrs | Very High | bug-137, bug-174, bug-178 |
| **Total** | **29-39 hrs** | | **4-5 tests** |

## Execution Strategy

1. Implement modules 1-7 sequentially in a single PR (they don't touch core parsing paths)
2. Module 8 as a separate PR (touches `parseChildren`, has compile-time implications)
3. After each module: build, run full test suite, fix regressions
4. After all modules: uncomment disabled tests, fix any failures

## Post-Implementation

- Uncomment disabled tests: bug-137, bug-174, bug-178 (func:function), add date_add
- Verify no regressions in existing passing tests
- Run: `go test ./xslt/ -run TestGeneral -count=1 -v`
- Update the gap analysis doc with completed Phase 8.4
