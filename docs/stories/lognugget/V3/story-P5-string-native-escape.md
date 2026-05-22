# P5 — appendJSONStringStr: String-Native JSON Escape

Owner-role: engineer.
Issue: [#116](https://github.com/architagr/LogNugget/issues/116).
Branch: `feat/116-p5-string-native-escape` (cut from `feat/111-v3-performance`, PRs back to `feat/111-v3-performance`).
Depends: none. Independent; may be worked in parallel with P1, P4, P6, P8.
LOC est: 100.

## Summary

`AppendQuotedString(dst []byte, s string)` calls `appendJSONString(dst, []byte(s))` — the `[]byte(s)` conversion allocates a copy of the string on every call. Add `appendJSONStringStr(dst []byte, s string) []byte` that operates on the string directly. Update all callers on the hot path to use the new function.

**Saving:** ~20 ns, ~2 allocs/event (one per string field: message, and any typed `KindStr` fields).

## Files touched

- `config/parse_field.go`:
  - Add `appendJSONStringStr(dst []byte, s string) []byte` — identical logic to `appendJSONString` but reads from `string` not `[]byte`
  - Update `AppendQuotedString(dst []byte, s string)` to call `appendJSONStringStr`
  - Update `AppendAttr` `KindStr` case to call `appendJSONStringStr`
  - Update `AppendField` `string` case to call `appendJSONStringStr`
- NEW `config/string_escape_bench_test.go` — `BenchmarkAppendQuotedString_ASCII`, `BenchmarkAppendQuotedString_Unicode`, `BenchmarkAppendQuotedString_Escape`

## appendJSONStringStr implementation

```go
func appendJSONStringStr(dst []byte, s string) []byte {
    dst = append(dst, '"')
    for i := 0; i < len(s); {
        b := s[i]
        if b >= utf8.RuneSelf {
            r, size := utf8.DecodeRuneInString(s[i:])
            if r == utf8.RuneError && size == 1 {
                dst = append(dst, '\xef', '\xbf', '\xbd')
                i++
                continue
            }
            dst = append(dst, s[i:i+size]...)
            i += size
            continue
        }
        switch cfgEscapeTable[b] {
        case 0:
            dst = append(dst, b)
        case cfgEscHex:
            dst = append(dst, '\\', 'u', '0', '0', cfgHexDigits[b>>4], cfgHexDigits[b&0xf])
        case cfgEscQuot:
            dst = append(dst, '\\', '"')
        case cfgEscBksl:
            dst = append(dst, '\\', '\\')
        case cfgEscB:
            dst = append(dst, '\\', 'b')
        case cfgEscF:
            dst = append(dst, '\\', 'f')
        case cfgEscN:
            dst = append(dst, '\\', 'n')
        case cfgEscR:
            dst = append(dst, '\\', 'r')
        case cfgEscT:
            dst = append(dst, '\\', 't')
        }
        i++
    }
    dst = append(dst, '"')
    return dst
}
```

Note: `appendJSONString(dst, src []byte)` is kept for the `error.Error()` and `time.Time.Format()` call sites that produce `[]byte` (or where the `[]byte` form is already available). Do not remove it.

## Tests-first gate

1. `Test_AppendJSONStringStr_MatchesByteVariant` — property test: for a large set of ASCII, unicode, and escape-requiring strings, `appendJSONStringStr(nil, s)` == `appendJSONString(nil, []byte(s))`.
2. `Test_AppendJSONStringStr_EmptyString` — returns `""` (two quote bytes).
3. `Test_AppendJSONStringStr_ControlChars` — `\t`, `\n`, `\r`, `"`, `\` are escaped correctly.
4. `Test_AppendJSONStringStr_Unicode` — multi-byte UTF-8 rune encoded correctly.
5. `Test_AppendJSONStringStr_InvalidUTF8` — invalid UTF-8 byte replaced with U+FFFD.
6. `BenchmarkAppendQuotedString_ASCII` — 0 allocs/op for a 20-char ASCII string when dst has capacity.

## Acceptance criteria

1. `AppendQuotedString(dst, s)` calls `appendJSONStringStr(dst, s)` — no `[]byte(s)` conversion inside.
2. `AppendAttr` `KindStr` case calls `appendJSONStringStr`.
3. `AppendField` `string` case calls `appendJSONStringStr`.
4. `Test_AppendJSONStringStr_MatchesByteVariant` passes with 1000+ inputs.
5. `BenchmarkAppendQuotedString_ASCII`: 0 allocs/op.
6. All existing escape tests (`Test_AppendField_StringEscape`, JSON encoder tests) pass unchanged.

## Assignment

Assigned to: agent-engineer
Branch: feat/116-p5-string-native-escape
Target PR: feat/111-v3-performance
Date: 2026-05-22
PL instruction: The new function is ~30 lines. Write the property test FIRST to ensure parity with the byte-slice variant. Do not touch `appendJSONString` — only add the new string variant and update callers.
