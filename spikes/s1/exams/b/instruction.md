src/tollgate/domain/periods.py currently derives only the calendar-month period start. ADR 0027 defers the rolling_days kind because it has no anchor to derive a window start from.

Add a pure function rolling_window_start(now, days, anchor) to that module, alongside calendar_month_start.

Semantics: the budget period began at anchor and repeats every `days` days. Return the start instant of the window that contains `now`, that is anchor advanced by whole multiples of `days` until the next advance would pass `now`. When now equals a window boundary exactly, that boundary is the window start.

Both now and anchor must be timezone-aware, and both convert to UTC before the arithmetic, matching how calendar_month_start handles zones. days must be a positive integer. now must not precede anchor. Reject each of these violations with a typed error from src/tollgate/domain/errors.py rather than a bare ValueError, consistent with that module contract that callers match on the type, never on a string; add a suitable error class there if none fits.

Keep the module pure: no I/O and no internal imports beyond the domain errors.

The verification tests are supplied separately, so change only the source under src/ — do not add or modify any test.
