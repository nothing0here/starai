package service

import "fmt"

func ValidateWorkbenchTheme(value interface{}) error {
	theme, ok := value.(string)
	if !ok || (theme != "dark" && theme != "light") {
		return fmt.Errorf("工作台默认主题只能是 dark（深色）或 light（浅色）")
	}
	return nil
}

func WorkbenchDefaultTheme(value interface{}) string {
	if theme, ok := value.(string); ok && theme == "light" {
		return "light"
	}
	return "dark"
}
