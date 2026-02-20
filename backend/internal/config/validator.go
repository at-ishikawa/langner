package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	enTranslations "github.com/go-playground/validator/v10/translations/en"
)

func newValidator() (*validator.Validate, ut.Translator, error) {
	validate := validator.New()

	enLocale := en.New()
	uni := ut.New(enLocale, enLocale)
	trans, _ := uni.GetTranslator("en")
	if err := enTranslations.RegisterDefaultTranslations(validate, trans); err != nil {
		return nil, nil, fmt.Errorf("failed to register default translations: %w", err)
	}

	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("mapstructure"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	if err := validate.RegisterValidation("file", isFileReadable); err != nil {
		return nil, nil, fmt.Errorf("failed to register file validation: %w", err)
	}
	if err := validate.RegisterTranslation("file", trans, func(ut ut.Translator) error {
		return ut.Add("file", "{0} must be an existing and readable file", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("file", strings.TrimPrefix(fe.Namespace(), "Config."))
		return t
	}); err != nil {
		return nil, nil, fmt.Errorf("failed to register file translation: %w", err)
	}

	return validate, trans, nil
}

func isFileReadable(fl validator.FieldLevel) bool {
	path := fl.Field().String()
	if path == "" {
		return false
	}

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	if info.IsDir() {
		return false
	}

	// Check if the owner has read permission
	return info.Mode().Perm()&(1<<(uint(7))) != 0
}
