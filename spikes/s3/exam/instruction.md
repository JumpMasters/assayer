Tollgate's cost model prices three token classes: input, output, and cached input. Add a fourth — cache creation (cache-write), the tokens a call spends writing into the provider's prompt cache. Anthropic bills these at their own rate, and today Tollgate does not count them at all, so every cached call is undercounted.

Thread one new price rate and one new usage count along the exact chain the existing `cached_input_*` fields already follow: price-book column, price repo, domain cost model, command and usage types, HTTP wire schemas, SDK, and the commit and grace handlers including their idempotency fingerprints.

Name them `cache_creation_micro_per_token` on `ModelPrice` and `cache_creation_tokens` on `ProviderUsage`.

Two things make this class behave unlike cached input, and both matter:

- **The rate may exceed the standard input rate.** Cache reads are cheaper than input and the model asserts that; cache creation is billed at a premium. The validation that rejects a cached-input rate above the input rate must not apply to this one. Rates stay non-negative, consistent with the existing check.
- **The count is disjoint and additive, not a subset.** `cached_input_tokens` is part of `input_tokens` and is priced instead of it. Cache-creation tokens are separate: price them as their own term added on top, and do not subtract them from the input count.

It is a reconcile-time class. Price it only where actual provider-reported usage is reconciled — the actual-cost path used by commit and grace — never in the worst-case reserve estimate, which has no way to know it in advance.

The usage count defaults to zero everywhere it appears, so every existing caller and every stored row keeps working untouched: a call that reports no cache-creation tokens costs exactly what it costs today, and the reserve estimate does not move at all.

The verification tests are supplied separately, so change only the source under `src/` and `migrations/` — do not add or modify any test.
