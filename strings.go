package vtui

// UIStrings holds default strings used by the UI framework itself.
// The application can overwrite these during initialization for localization.
var UIStrings = struct {
	DesktopWelcome string
	ButtonBrackets [2]rune
	DefaultHelp    string
}{
	DesktopWelcome: " vtui | Press Ctrl+Q to exit ",
	ButtonBrackets: [2]rune{'[', ']'},
	DefaultHelp:    "Contents",
}
