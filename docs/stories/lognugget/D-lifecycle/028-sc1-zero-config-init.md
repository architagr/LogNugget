# 028 — SC1 + F20: zero-config init wiring

Owner-role: engineer.
Refs: SC1, F20, TS-24 (zero_config half).
Depends: 027.
LOC est: 220.

## Files touched

- `lognugget/lognugget.go` (`init` body: construct default post-proc with rate=1s, max=20, output=os.Stdout per LLD §9.1; register on LevelUnSet; `config.InitPreProcessors`)
- NEW `lognugget/zero_config_test.go` (TS-24 SC1 half)
- UPDATE `testdata/golden/zero_config.json`

## Tests-first gate

1. `Test_ZeroConfig_EmitsJSONLineOnStdout` — capture os.Stdout via support helper; `entry.NewLogEntry().Info(ctx, "msg")`; assert one line; `json.Unmarshal` succeeds; ends with `\n`.
2. `Test_ZeroConfig_GoldenMaster` — with FakeClock pinned, output matches `testdata/golden/zero_config.json`.
3. `Test_ZeroConfig_NoCallerByDefault` — confirms 012 (addSource off default) holds end-to-end.

## Acceptance

1. `lognugget.init` wires defaults per LLD §9.1.
2. SC1 green.
3. Golden file populated with real bytes.
4. Default rate=1s, max=20 documented (N-2).

## Bench notes

- Init runs once.
- Hot path: ensure `lognugget` package import doesn't pull anything into render path.
