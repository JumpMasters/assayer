In src/tollgate/domain/money.py, the _checked helper raises a bare ValueError when an amount is negative, but raises the typed AmountOutOfRange when it is too large. That is inconsistent with the contract stated in src/tollgate/domain/errors.py, which says callers match on the type, never on a string.

Add an InvalidAmount error to src/tollgate/domain/errors.py and raise it for the negative case. It must subclass both TollgateError and ValueError, so existing callers that catch ValueError keep working.

The verification tests are supplied separately, so change only the source under src/ — do not add or modify any test.
