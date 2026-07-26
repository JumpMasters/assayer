src/tollgate/domain/errors.py states that these errors describe business and operational outcomes, and that callers match on the type, never on a string. Every error defined in that module subclasses TollgateError.

The domain layer does not hold to that contract. Input validation in that layer still rejects bad values by raising a bare ValueError, so a caller that catches TollgateError misses those failures entirely.

Find every place in the domain layer where that happens and bring it into line with the contract.

Every existing test must continue to pass unchanged; a number of them assert on ValueError today.

The verification tests are supplied separately, so change only the source under src/ — do not add or modify any test.
