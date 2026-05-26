package tool

import (
	"mvdan.cc/sh/v3/syntax"

	"github.com/sageil/kodacode/internal/workspace"
)

func (a *bashPathAnalyzer) visitStmtList(stmts []*syntax.Stmt) bool {
	changed := false
	for _, stmt := range stmts {
		if a.visitStmt(stmt) {
			changed = true
		}
	}
	return changed
}

func (a *bashPathAnalyzer) visitStmt(stmt *syntax.Stmt) bool {
	if stmt == nil {
		return false
	}
	if stmt.Background || stmt.Coprocess || stmt.Disown {
		return a.visitIsolatedStmt(stmt)
	}
	for _, redirect := range stmt.Redirs {
		a.visitRedirect(redirect)
	}
	return a.visitCommand(stmt.Cmd)
}

func (a *bashPathAnalyzer) visitIsolatedStmt(stmt *syntax.Stmt) bool {
	if stmt == nil {
		return false
	}
	clone := a.fork()
	for _, redirect := range stmt.Redirs {
		clone.visitRedirect(redirect)
	}
	_ = clone.visitCommand(stmt.Cmd)
	a.merge(clone)
	return false
}

func (a *bashPathAnalyzer) visitCommand(cmd syntax.Command) bool {
	switch typed := cmd.(type) {
	case nil:
		return false
	case *syntax.CallExpr:
		return a.visitCallExpr(typed)
	case *syntax.BinaryCmd:
		return a.visitBinaryCmd(typed)
	case *syntax.Block:
		return a.visitStmtList(typed.Stmts)
	case *syntax.Subshell:
		return a.visitIsolatedStmtList(typed.Stmts)
	case *syntax.IfClause:
		groups := [][]*syntax.Stmt{typed.Cond, typed.Then}
		groups = append(groups, ifElseStmtLists(typed.Else)...)
		return a.visitConditionalStmtLists(groups...)
	case *syntax.WhileClause:
		return a.visitConditionalStmtLists(typed.Cond, typed.Do)
	case *syntax.ForClause:
		return a.visitConditionalStmtLists(typed.Do)
	case *syntax.CaseClause:
		groups := make([][]*syntax.Stmt, 0, len(typed.Items))
		for _, item := range typed.Items {
			if item == nil {
				continue
			}
			groups = append(groups, item.Stmts)
		}
		return a.visitConditionalStmtLists(groups...)
	case *syntax.TimeClause:
		return a.visitStmt(typed.Stmt)
	case *syntax.CoprocClause:
		return a.visitIsolatedStmt(typed.Stmt)
	case *syntax.FuncDecl:
		return false
	default:
		return false
	}
}

func (a *bashPathAnalyzer) visitIsolatedStmtList(stmts []*syntax.Stmt) bool {
	clone := a.fork()
	_ = clone.visitStmtList(stmts)
	a.merge(clone)
	return false
}

func (a *bashPathAnalyzer) visitConditionalStmtLists(groups ...[]*syntax.Stmt) bool {
	changed := false
	for _, stmts := range groups {
		if len(stmts) == 0 {
			continue
		}
		clone := a.fork()
		if clone.visitStmtList(stmts) {
			changed = true
		}
		a.merge(clone)
	}
	if changed {
		a.setOpaquePathReason("command changes the working directory inside shell control flow")
	}
	return false
}

func ifElseStmtLists(branch *syntax.IfClause) []([]*syntax.Stmt) {
	if branch == nil {
		return nil
	}
	groups := [][]*syntax.Stmt{}
	current := branch
	for current != nil {
		if len(current.Cond) > 0 {
			groups = append(groups, current.Cond)
		}
		if len(current.Then) > 0 {
			groups = append(groups, current.Then)
		}
		if current.ThenPos.IsValid() {
			current = current.Else
			continue
		}
		break
	}
	return groups
}

func (a *bashPathAnalyzer) visitBinaryCmd(cmd *syntax.BinaryCmd) bool {
	if cmd == nil {
		return false
	}
	switch cmd.Op {
	case syntax.AndStmt:
		leftChanged := a.visitStmt(cmd.X)
		rightChanged := a.visitStmt(cmd.Y)
		return leftChanged || rightChanged
	case syntax.OrStmt:
		originalDir := a.workingDir
		left := a.fork()
		leftChanged := left.visitStmt(cmd.X)
		a.merge(left)
		right := a.fork()
		rightChanged := right.visitStmt(cmd.Y)
		a.merge(right)
		if leftChanged || rightChanged {
			a.setOpaquePathReason("command changes the working directory inside shell control flow")
		}
		a.workingDir = originalDir
		return false
	case syntax.Pipe, syntax.PipeAll:
		_ = a.visitIsolatedStmt(cmd.X)
		_ = a.visitIsolatedStmt(cmd.Y)
		return false
	default:
		leftChanged := a.visitStmt(cmd.X)
		rightChanged := a.visitStmt(cmd.Y)
		return leftChanged || rightChanged
	}
}

func (a *bashPathAnalyzer) visitCallExpr(call *syntax.CallExpr) bool {
	if call == nil || len(call.Args) == 0 {
		return false
	}
	commandName, literal := shellWordLiteral(call.Args[0])
	if literal {
		if isShellChangeDirectoryBuiltin(commandName) {
			literalArgs, ok := shellLiteralCallArgs(call)
			if !ok {
				a.setOpaquePathReason("command changes the working directory inside the shell")
				return false
			}
			if nextDir, ok := resolveLiteralShellWorkingDir(a.workingDir, literalArgs[1:]); ok {
				currentDir := canonicalShellWorkingDir(a.workingDir)
				nextDir = canonicalShellWorkingDir(nextDir)
				a.addRequest(workspace.AccessWorkdir, nextDir, "command changes the working directory to a filesystem path")
				changed := nextDir != currentDir
				a.workingDir = nextDir
				return changed
			}
			a.setOpaquePathReason("command changes the working directory inside the shell")
			return false
		}
		if isDirectoryChangingShellBuiltin(commandName) {
			a.setOpaquePathReason("command changes the working directory inside the shell")
			return false
		}
		if path, ok := shellLiteralPathCandidate(commandName); ok {
			if resolved, ok := resolveShellPath(a.workingDir, path); ok {
				a.addRequest(workspace.AccessExec, resolved, "command executes a filesystem path")
			}
		}
	} else if shellWordHasDynamicExpansion(call.Args[0]) {
		a.setOpaquePathReason("command name includes dynamic shell expansion")
	}

	for _, arg := range call.Args[1:] {
		a.visitArgument(arg)
	}
	return false
}

func (a *bashPathAnalyzer) visitArgument(word *syntax.Word) {
	if word == nil {
		return
	}
	if literal, ok := shellWordLiteral(word); ok {
		if path, match := shellArgumentPath(a.workingDir, literal); match {
			a.addRequest(workspace.AccessRead, path, "command references a filesystem path")
		}
		return
	}
	if shellWordHasDynamicExpansion(word) {
		a.setOpaquePathReason("command includes dynamic shell expansion that may change accessed paths")
	}
}

func (a *bashPathAnalyzer) visitRedirect(redirect *syntax.Redirect) {
	if redirect == nil || redirect.Word == nil {
		return
	}
	access := redirectAccess(redirect)
	if literal, ok := shellWordLiteral(redirect.Word); ok {
		if path, match := shellArgumentPath(a.workingDir, literal); match {
			reason := "command redirects output to a filesystem path"
			if access == workspace.AccessRead {
				reason = "command redirects input from a filesystem path"
			}
			a.addRequest(access, path, reason)
		}
		return
	}
	if shellWordHasDynamicExpansion(redirect.Word) {
		a.setOpaquePathReason("command includes dynamic redirect path expansion")
	}
}
