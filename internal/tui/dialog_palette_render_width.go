package tui

const (
	commandPaletteDefaultModalWidth  = 70
	commandPaletteDefaultModalHeight = 20
)

func (d *commandPaletteDialog) defaultModalWidth() int {
	switch d.kind {
	case commandPaletteActions:
		return 132
	case commandPaletteModel:
		return 136
	case commandPaletteAgent:
		return 72
	case commandPaletteUtilityModel, commandPaletteReviewerModel:
		return 136
	default:
		return commandPaletteDefaultModalWidth
	}
}

func (d *commandPaletteDialog) dialogWidth() int {
	return desiredDialogWidth(d.frameWidth, d.minWidth(), d.defaultModalWidth())
}

func (d *commandPaletteDialog) dialogContentMinBodyHeight() int {
	return 0
}

func (d *commandPaletteDialog) minWidth() int {
	switch d.kind {
	case commandPaletteActions, commandPaletteUtilityModel, commandPaletteReviewerModel:
		return 48
	case commandPaletteModel:
		return 48
	case commandPaletteAgent:
		return 32
	default:
		return 32
	}
}
