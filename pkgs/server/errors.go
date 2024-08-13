package main

type JigError struct {
	description string
	isCritical  bool
}

func (e *JigError) Error() string {
	return e.description
}
