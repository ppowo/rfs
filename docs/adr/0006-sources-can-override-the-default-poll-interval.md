# Sources can override the default poll interval

rfs originally polled every Source in one cycle at the process-wide `-interval`, which defaults to one hour. The Will Codex Reset forecast refreshes about every 30 minutes, so the global cadence would delay its alert feed or require polling every unrelated Source twice as often.

A Source may now set `Source.Interval` to a positive duration. Sources with no override continue to use `-interval`. At startup rfs groups Sources by their effective interval and runs one draining `Loop` per schedule; Sources in the same schedule are still polled concurrently, and all schedules share the HTTP client and domain gate.

## Consequences

- The Codex quota Source polls every 30 minutes while existing Sources retain the one-hour default.
- `-interval` is the default for Sources without an override rather than an unconditional global cadence.
- Shutdown waits for every poll schedule to drain before exiting or replacing the process during self-update.
