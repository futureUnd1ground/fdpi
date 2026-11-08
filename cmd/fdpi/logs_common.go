package main

import "fmt"

func errGrepArg() error {
	return fmt.Errorf("после --grep укажите текст поиска")
}
func errCategoryArg() error { return fmt.Errorf("--category err|warning|info") }
func errBadCategory() error {
	return fmt.Errorf("неверная категория журнала: используйте err, warning или info")
}
func errUnknownLogArg() error {
	return fmt.Errorf("неизвестный параметр журнала")
}
