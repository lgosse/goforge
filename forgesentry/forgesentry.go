// Package forgesentry connects structured slog records to Sentry.
//
// It decorates an existing slog handler, preserves normal log delivery, and
// reports error-level records with their structured attributes and underlying
// errors attached.
package forgesentry
