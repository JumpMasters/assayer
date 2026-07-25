src/tollgate/domain/errors.py states that these errors describe business and operational outcomes and that callers match on the type, never on a string. Every error in that module subclasses TollgateError.

src/tollgate/domain/pricing.py does not follow that contract. It validates its inputs in four places — ModelPrice.__post_init__, estimate_micro, actual_micro, and reconcile — and every one of them raises a bare ValueError.

Bring pricing.py in line with the contract so its validation failures are typed domain errors.

Every existing test must continue to pass unchanged; several of them assert on ValueError today.

The verification tests are supplied separately, so change only the source under src/ — do not add or modify any test.
