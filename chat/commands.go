package chat

// SlashCommandConfig implements a named slash command (Config.SlashCommands keys are matched
// case-insensitively against the parsed command name).
type SlashCommandConfig interface {
	Handle(app *App, sc SlashCommand) (SlashResponse, error)
}

type TransformCommand struct {
	Transform func(sc SlashCommand) (string, error)
}

func (t *TransformCommand) Handle(app *App, sc SlashCommand) (SlashResponse, error) {
	res, err := t.Transform(sc)
	if err != nil {
		return SlashResponse{}, err
	}
	return sc.NewResponse(res), nil
}

type ExecuteCommand struct {
	Execute func(app *App, sc SlashCommand) error
}

func (e *ExecuteCommand) Handle(app *App, sc SlashCommand) (SlashResponse, error) {
	err := e.Execute(app, sc)
	if err != nil {
		return SlashResponse{}, err
	}
	return sc.Handled(), nil
}
