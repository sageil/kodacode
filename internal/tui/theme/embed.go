package theme

import "embed"

//go:embed themes/*.yaml
var builtinThemeFS embed.FS

func BuiltinThemeFS() embed.FS { return builtinThemeFS }
