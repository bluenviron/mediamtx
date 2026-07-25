package logger

// Writer is an object that provides a log method.
type Writer interface {
	Log(Level, string, ...any)
}

// InlineWriter is a Writer that can be created inline.
type InlineWriter struct {
	Parent Writer
	Prefix string
}

// Log implements Writer.
func (w *InlineWriter) Log(level Level, msg string, args ...any) {
	w.Parent.Log(level, w.Prefix+" "+msg, args...)
}
