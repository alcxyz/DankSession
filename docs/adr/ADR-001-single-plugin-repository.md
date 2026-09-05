# ADR-001: Keep the DMS interface and Go backend in one repository

**Status:** Accepted

## Decision

DankSession is a standalone DMS plugin repository containing its QML interface, Go backend, state schema, tests, and packaging. They form one product and use one version.

## Consequences

The widget and backend cannot drift across releases. Backend changes must continue to work without the QML process because restoration begins with the graphical session.
