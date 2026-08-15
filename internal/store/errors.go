package store

// InvalidInputError reports that a store call was rejected because the
// caller supplied an invalid input. It is distinct from provider error
// classes: the API maps it to the InvalidRequest 400 error envelope.
type InvalidInputError struct {
	Message string
}

func (e InvalidInputError) Error() string {
	return e.Message
}
