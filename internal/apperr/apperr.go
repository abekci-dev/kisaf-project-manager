// Package apperr gives user-facing errors a stable machine-readable code
// alongside their English text.
//
// Localisation happens in the browser, not here. The server would otherwise
// have to know each request's language and carry a second message catalogue,
// while the UI already owns one for its own labels. Sending a code lets the UI
// translate, and the English text stays the answer for curl, logs, and any
// language the UI has not been translated into yet.
package apperr

import "fmt"

// Error is a user-facing failure with a translation key.
type Error struct {
	// Code is a dotted identifier the UI can look up, e.g. "project.duplicate".
	Code string
	// Msg is the English fallback, already formatted.
	Msg string
	// Args are the substitutions the UI needs to rebuild the message in another
	// language — a folder name, a limit, and so on.
	Args []any
	// Err is an optional wrapped cause.
	Err error
}

func (e *Error) Error() string { return e.Msg }
func (e *Error) Unwrap() error { return e.Err }

// New builds an Error. format is the English message; args are both formatted
// into it and passed through to the client for re-formatting.
func New(code, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...), Args: args}
}

// Wrap builds an Error that keeps an underlying cause for errors.Is/As.
func Wrap(err error, code, format string, args ...any) *Error {
	e := New(code, format, args...)
	e.Err = err
	return e
}

// Codes used across the application. They are constants so a typo in one place
// becomes a compile error rather than a silently untranslated message.
const (
	CodeProjectNotFound  = "project.notFound"
	CodeProjectDuplicate = "project.duplicate"
	CodeProjectNotDir    = "project.notDir"
	CodeProjectUnopened  = "project.folderUnreadable"
	CodePathEmpty        = "path.empty"
	CodePathInvalid      = "path.invalid"
	CodePathOutside      = "path.outside"

	CodeTodoNotFound = "todo.notFound"
	CodeTodoEmpty    = "todo.empty"
	CodeTodoLimit    = "todo.limit"

	CodeEditorNotFound   = "editor.notFound"
	CodeEditorNotChosen  = "editor.notChosen"
	CodeTerminalNotFound = "terminal.notFound"
	CodeFileManagerNone  = "fileManager.notFound"
	CodeUnknownAction    = "action.unknown"

	CodeFileNotFound    = "file.notFound"
	CodeFileBinary      = "file.binary"
	CodeDirUnreadable   = "dir.unreadable"
	CodeRequestInvalid  = "request.invalid"
	CodeScanFailed      = "scan.failed"
	CodeGitUnavailable  = "git.unavailable"
	CodeRemoteDisabled  = "auth.remoteDisabled"
	CodeAuthRequired    = "auth.required"
	CodeOriginRejected  = "origin.rejected"
	CodeHostRejected    = "host.rejected"
	CodeQuitLocalOnly   = "quit.localOnly"
	CodeQuitBadRequest  = "quit.badRequest"
	CodeSettingsInvalid = "settings.invalid"
	CodeInternal        = "internal"
)
