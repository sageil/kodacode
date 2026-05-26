package tool

import (
	"path/filepath"
	"strings"
)

type plannedTestCommand struct {
	Framework       testFramework
	Command         string
	WorkingDir      string
	ProjectRoot     string
	PathSpecified   bool
	FilterApplied   bool
	TargetApplied   bool
	ExplicitCommand bool
}

func planTestCommand(workspaceRoot string, input testInput, target testTarget) (plannedTestCommand, error) {
	if strings.TrimSpace(input.Command) != "" {
		workingDir := workspaceRoot
		if target.Specified {
			workingDir = target.StartDir
			if !target.IsFile {
				workingDir = target.ResolvedPath
			}
		}
		command, err := applyTestFilter(input.Command, input.Filter, frameworkFromCommand(input.Command))
		if err != nil {
			return plannedTestCommand{}, err
		}
		planned := plannedTestCommand{
			Framework:       frameworkFromCommand(input.Command),
			Command:         command,
			WorkingDir:      workingDir,
			ProjectRoot:     workingDir,
			PathSpecified:   target.Specified,
			FilterApplied:   strings.TrimSpace(input.Filter) != "",
			ExplicitCommand: true,
		}
		return validateOneShotTestCommand(planned)
	}

	detected, err := detectTestFramework(workspaceRoot, target)
	if err != nil {
		return plannedTestCommand{}, err
	}
	command, targetApplied, err := applyTestTarget(detected.Command, detected.Framework, detected.RootDir, target)
	if err != nil {
		return plannedTestCommand{}, err
	}
	command, err = applyTestFilter(command, input.Filter, detected.Framework)
	if err != nil {
		return plannedTestCommand{}, err
	}
	planned := plannedTestCommand{
		Framework:       detected.Framework,
		Command:         command,
		WorkingDir:      detected.RootDir,
		ProjectRoot:     detected.RootDir,
		PathSpecified:   target.Specified,
		FilterApplied:   strings.TrimSpace(input.Filter) != "",
		TargetApplied:   targetApplied,
		ExplicitCommand: false,
	}
	return validateOneShotTestCommand(planned)
}

func validateOneShotTestCommand(plan plannedTestCommand) (plannedTestCommand, error) {
	intentAnalysis, err := analyzeBashIntent(plan.WorkingDir, plan.Command)
	if err != nil {
		return plannedTestCommand{}, err
	}
	switch intentAnalysis.Intent {
	case ExecutionIntentWatcher, ExecutionIntentServer:
		return plannedTestCommand{}, ErrTestWatchModeUnsupported
	}
	return plan, nil
}

func applyTestTarget(command string, framework testFramework, projectRoot string, target testTarget) (string, bool, error) {
	if !target.Specified {
		return command, false, nil
	}
	relative, err := filepath.Rel(projectRoot, target.ResolvedPath)
	if err != nil {
		return "", false, err
	}
	relative = filepath.ToSlash(relative)
	if relative == "." {
		return command, false, nil
	}
	quoted := shellQuote(relative)
	switch framework {
	case testFrameworkGo:
		dir := relative
		if target.IsFile {
			dir = filepath.ToSlash(filepath.Dir(relative))
		}
		if dir == "." || dir == "" {
			return "go test .", true, nil
		}
		return "go test ./" + dir, true, nil
	case testFrameworkPytest, testFrameworkRspec, testFrameworkPHPUnit:
		return command + " " + quoted, true, nil
	case testFrameworkNodeVitest, testFrameworkNodeJest, testFrameworkNodeMocha, testFrameworkNodePlaywright:
		return appendNodeTestArgs(command, quoted), true, nil
	case testFrameworkTask, testFrameworkMake, testFrameworkCargo, testFrameworkMaven, testFrameworkGradle, testFrameworkDotnet, testFrameworkElixir, testFrameworkZig:
		return "", false, ErrTestPathTargetUnsupported
	default:
		return "", false, ErrTestPathTargetUnsupported
	}
}

func applyTestFilter(command, filter string, framework testFramework) (string, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return command, nil
	}
	quoted := shellQuote(filter)
	switch framework {
	case testFrameworkGo:
		if strings.Contains(command, " -run ") {
			return command, nil
		}
		return command + " -run " + quoted, nil
	case testFrameworkPytest:
		if strings.Contains(command, " -k ") {
			return command, nil
		}
		return command + " -k " + quoted, nil
	case testFrameworkCargo:
		return command + " " + quoted, nil
	case testFrameworkMaven:
		if strings.Contains(command, "-Dtest=") {
			return command, nil
		}
		return command + " -Dtest=" + quoted, nil
	case testFrameworkGradle:
		if strings.Contains(command, " --tests ") {
			return command, nil
		}
		return command + " --tests " + quoted, nil
	case testFrameworkDotnet:
		if strings.Contains(command, " --filter ") {
			return command, nil
		}
		return command + " --filter " + quoted, nil
	case testFrameworkRspec:
		if strings.Contains(command, " --example ") {
			return command, nil
		}
		return command + " --example " + quoted, nil
	case testFrameworkPHPUnit:
		if strings.Contains(command, " --filter ") {
			return command, nil
		}
		return command + " --filter " + quoted, nil
	case testFrameworkNodeVitest, testFrameworkNodeJest:
		if strings.Contains(command, " -t ") || strings.Contains(command, " --testNamePattern ") {
			return command, nil
		}
		return appendNodeTestArgs(command, "-t", quoted), nil
	case testFrameworkNodeMocha, testFrameworkNodePlaywright:
		if strings.Contains(command, " --grep ") {
			return command, nil
		}
		return appendNodeTestArgs(command, "--grep", quoted), nil
	default:
		return "", ErrTestFilterUnsupported
	}
}

func appendNodeTestArgs(command string, args ...string) string {
	command = strings.TrimSpace(command)
	switch {
	case strings.HasPrefix(command, "npm test"),
		strings.HasPrefix(command, "pnpm test"),
		strings.HasPrefix(command, "yarn test"),
		strings.HasPrefix(command, "bun run test"):
		if strings.Contains(command, " -- ") {
			return command + " " + strings.Join(args, " ")
		}
		return command + " -- " + strings.Join(args, " ")
	default:
		return command + " " + strings.Join(args, " ")
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
