package search

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

type goSymbolExtractor struct{}

func (goSymbolExtractor) Name() string { return "go-ast" }

func (goSymbolExtractor) Supports(language, path string) bool {
	return DetectLanguage(path, language) == "go"
}

func (goSymbolExtractor) Extract(_ context.Context, path string) ([]Symbol, error) {
	return ExtractGoSymbols(path), nil
}

type goSymbolEnricher struct{}

func (goSymbolEnricher) Name() string { return "go-ast-docs" }

func (goSymbolEnricher) Supports(language, path string) bool {
	return DetectLanguage(path, language) == "go"
}

func (goSymbolEnricher) Enrich(path string, symbols []Symbol) []Symbol {
	return EnrichGoSymbols(path, symbols)
}

func parseGoFile(filePath string) (*token.FileSet, *ast.File, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}
	return fset, f, nil
}

// ExtractGoSymbols parses a Go source file and builds a symbol list directly
// from the AST. It is used when ctags is unavailable.
func ExtractGoSymbols(filePath string) []Symbol {
	fset, f, err := parseGoFile(filePath)
	if err != nil {
		return nil
	}

	var symbols []Symbol
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := Symbol{
				FilePath:  filePath,
				Name:      d.Name.Name,
				Kind:      "function",
				Language:  "go",
				Signature: funcSignature(d),
				Line:      fset.Position(d.Pos()).Line,
				Parent:    receiverTypeName(d),
				Tokens:    SplitTokens(d.Name.Name),
			}
			if d.Doc != nil {
				sym.Doc = strings.TrimSpace(d.Doc.Text())
			}
			symbols = append(symbols, sym)
		case *ast.GenDecl:
			switch d.Tok {
			case token.TYPE:
				for _, spec := range d.Specs {
					s, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					doc := d.Doc
					if s.Doc != nil {
						doc = s.Doc
					}
					kind := "type"
					if _, ok := s.Type.(*ast.InterfaceType); ok {
						kind = "interface"
					}
					sym := Symbol{
						FilePath: filePath,
						Name:     s.Name.Name,
						Kind:     kind,
						Language: "go",
						Line:     fset.Position(s.Pos()).Line,
						Tokens:   SplitTokens(s.Name.Name),
					}
					if doc != nil {
						sym.Doc = strings.TrimSpace(doc.Text())
					}
					symbols = append(symbols, sym)
				}
			case token.CONST, token.VAR:
				kind := "variable"
				if d.Tok == token.CONST {
					kind = "const"
				}
				for _, spec := range d.Specs {
					s, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					doc := d.Doc
					if s.Doc != nil {
						doc = s.Doc
					}
					for _, name := range s.Names {
						sym := Symbol{
							FilePath: filePath,
							Name:     name.Name,
							Kind:     kind,
							Language: "go",
							Line:     fset.Position(name.Pos()).Line,
							Tokens:   SplitTokens(name.Name),
						}
						if doc != nil {
							sym.Doc = strings.TrimSpace(doc.Text())
						}
						symbols = append(symbols, sym)
					}
				}
			}
		}
	}

	return symbols
}

// EnrichGoSymbols parses a Go source file and enriches matching symbols
// with doc comments and full signatures from the AST.
func EnrichGoSymbols(filePath string, symbols []Symbol) []Symbol {
	fset, f, err := parseGoFile(filePath)
	if err != nil {
		return symbols
	}

	byLine := make(map[int]*Symbol, len(symbols))
	for i := range symbols {
		byLine[symbols[i].Line] = &symbols[i]
	}

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			line := fset.Position(d.Pos()).Line
			if sym, ok := byLine[line]; ok {
				if d.Doc != nil {
					sym.Doc = strings.TrimSpace(d.Doc.Text())
				}
				sym.Signature = funcSignature(d)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				line := fset.Position(spec.Pos()).Line
				sym, ok := byLine[line]
				if !ok {
					continue
				}
				switch s := spec.(type) {
				case *ast.TypeSpec:
					doc := d.Doc
					if s.Doc != nil {
						doc = s.Doc
					}
					if doc != nil {
						sym.Doc = strings.TrimSpace(doc.Text())
					}
				case *ast.ValueSpec:
					doc := d.Doc
					if s.Doc != nil {
						doc = s.Doc
					}
					if doc != nil {
						sym.Doc = strings.TrimSpace(doc.Text())
					}
				}
			}
		}
	}

	return symbols
}

func receiverTypeName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return ""
	}
	return exprTypeName(d.Recv.List[0].Type)
}

func exprTypeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return exprTypeName(e.X)
	case *ast.IndexExpr:
		return exprTypeName(e.X)
	case *ast.IndexListExpr:
		return exprTypeName(e.X)
	case *ast.ParenExpr:
		return exprTypeName(e.X)
	case *ast.SelectorExpr:
		return e.Sel.Name
	default:
		return ""
	}
}

func funcSignature(d *ast.FuncDecl) string {
	var sb strings.Builder
	sb.WriteString("func ")

	if d.Recv != nil && len(d.Recv.List) > 0 {
		sb.WriteString("(")
		writeFieldList(&sb, d.Recv)
		sb.WriteString(") ")
	}

	sb.WriteString(d.Name.Name)
	sb.WriteString("(")
	if d.Type.Params != nil {
		writeFieldList(&sb, d.Type.Params)
	}
	sb.WriteString(")")

	if d.Type.Results != nil && len(d.Type.Results.List) > 0 {
		results := d.Type.Results
		if len(results.List) == 1 && len(results.List[0].Names) == 0 {
			sb.WriteString(" ")
			writeExpr(&sb, results.List[0].Type)
		} else {
			sb.WriteString(" (")
			writeFieldList(&sb, results)
			sb.WriteString(")")
		}
	}

	return sb.String()
}

func writeFieldList(sb *strings.Builder, fl *ast.FieldList) {
	for i, field := range fl.List {
		if i > 0 {
			sb.WriteString(", ")
		}
		for j, name := range field.Names {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(name.Name)
		}
		if len(field.Names) > 0 {
			sb.WriteString(" ")
		}
		writeExpr(sb, field.Type)
	}
}

func writeExpr(sb *strings.Builder, expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.Ident:
		sb.WriteString(e.Name)
	case *ast.SelectorExpr:
		writeExpr(sb, e.X)
		sb.WriteString(".")
		sb.WriteString(e.Sel.Name)
	case *ast.StarExpr:
		sb.WriteString("*")
		writeExpr(sb, e.X)
	case *ast.ArrayType:
		sb.WriteString("[]")
		writeExpr(sb, e.Elt)
	case *ast.MapType:
		sb.WriteString("map[")
		writeExpr(sb, e.Key)
		sb.WriteString("]")
		writeExpr(sb, e.Value)
	case *ast.InterfaceType:
		sb.WriteString("interface{}")
	case *ast.FuncType:
		sb.WriteString("func(")
		if e.Params != nil {
			writeFieldList(sb, e.Params)
		}
		sb.WriteString(")")
		if e.Results != nil && len(e.Results.List) > 0 {
			sb.WriteString(" ")
			if len(e.Results.List) == 1 && len(e.Results.List[0].Names) == 0 {
				writeExpr(sb, e.Results.List[0].Type)
			} else {
				sb.WriteString("(")
				writeFieldList(sb, e.Results)
				sb.WriteString(")")
			}
		}
	case *ast.Ellipsis:
		sb.WriteString("...")
		writeExpr(sb, e.Elt)
	case *ast.ChanType:
		switch e.Dir {
		case ast.SEND:
			sb.WriteString("chan<- ")
		case ast.RECV:
			sb.WriteString("<-chan ")
		default:
			sb.WriteString("chan ")
		}
		writeExpr(sb, e.Value)
	default:
		sb.WriteString("?")
	}
}
