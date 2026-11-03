package secrets

type maskedError struct {
	original error
	text     string
}

func (e maskedError) Error() string { return e.text }
func (e maskedError) Unwrap() error { return e.original }
func MaskError(err error) error {
	if err == nil {
		return nil
	}
	text := Masker().String(err.Error())
	if text == err.Error() {
		return err
	}
	return maskedError{original: err, text: text}
}
