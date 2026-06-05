package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var ErrInvalidArguments = errors.New("tool arguments are invalid")

type InvalidArgumentsError struct {
	ToolName string
	Message  string
	Cause    error
}

type contextualArgumentDetailError struct {
	Detail string
	Cause  error
}

func (e *InvalidArgumentsError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *InvalidArgumentsError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *InvalidArgumentsError) Is(target error) bool {
	return target == ErrInvalidArguments
}

func DecodeArgs(toolName string, args json.RawMessage, dst any) error {
	if err := json.Unmarshal(args, dst); err != nil {
		err = withArgumentDetail(args, err)
		return NormalizeArgumentError(toolName, err)
	}
	return nil
}

func DecodeArgsStrict(toolName string, args json.RawMessage, dst any, allowedFields ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err != nil {
		err = withArgumentDetail(args, err)
		return NormalizeArgumentError(toolName, err)
	}

	allowed := make(map[string]struct{}, len(allowedFields))
	for _, field := range allowedFields {
		allowed[field] = struct{}{}
	}
	var unknown []string
	for field := range fields {
		if _, ok := allowed[field]; !ok && !caseInsensitiveFieldAllowed(field, allowed) {
			unknown = append(unknown, field)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return InvalidArguments(toolName, fmt.Errorf("unknown field %q", unknown[0]))
	}

	return DecodeArgs(toolName, args, dst)
}

func caseInsensitiveFieldAllowed(field string, allowed map[string]struct{}) bool {
	for allowedField := range allowed {
		if strings.EqualFold(field, allowedField) {
			return true
		}
	}
	return false
}

func (e *contextualArgumentDetailError) Error() string {
	if e == nil {
		return ""
	}
	return strings.TrimSpace(e.Detail)
}

func (e *contextualArgumentDetailError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func decodeOptionalBoolArg(toolName string, raw json.RawMessage, field string) (bool, bool, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return false, false, nil
	}

	var value bool
	if err := json.Unmarshal(raw, &value); err == nil {
		return value, true, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		trimmed := strings.TrimSpace(text)
		switch {
		case strings.EqualFold(trimmed, "null"):
			return false, false, nil
		case strings.EqualFold(trimmed, "true"):
			return true, true, nil
		case strings.EqualFold(trimmed, "false"):
			return false, true, nil
		default:
			return false, false, InvalidArguments(toolName, fmt.Errorf("%s must be a boolean", field))
		}
	}

	return false, false, InvalidArguments(toolName, fmt.Errorf("%s must be a boolean", field))
}

func decodeOptionalIntArg(toolName string, raw json.RawMessage, field string) (int, bool, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return 0, false, nil
	}

	var value int
	if err := json.Unmarshal(raw, &value); err == nil {
		return value, true, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		trimmed := strings.TrimSpace(text)
		if strings.EqualFold(trimmed, "null") {
			return 0, false, nil
		}
		if trimmed == "" {
			return 0, false, InvalidArguments(toolName, fmt.Errorf("%s must be an integer", field))
		}
		value, err := strconv.Atoi(trimmed)
		if err != nil {
			return 0, false, InvalidArguments(toolName, fmt.Errorf("%s must be an integer", field))
		}
		return value, true, nil
	}

	return 0, false, InvalidArguments(toolName, fmt.Errorf("%s must be an integer", field))
}

func decodeOptionalStringArg(toolName string, raw json.RawMessage, field string) (string, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", InvalidArguments(toolName, fmt.Errorf("%s must be a string", field))
	}
	if strings.EqualFold(strings.TrimSpace(value), "null") {
		return "", nil
	}
	return value, nil
}

func hasNonNullRawJSON(raw json.RawMessage) bool {
	return len(raw) > 0 && strings.TrimSpace(string(raw)) != "null"
}

func decodeOptionalStringArrayArg(toolName string, raw json.RawMessage, field string) ([]string, bool, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, false, nil
	}

	items, err := decodeStringArrayItems(raw)
	if err != nil {
		return nil, false, invalidStringArrayTypeError(toolName, field, raw)
	}

	values := make([]string, 0, len(items))
	for idx, item := range items {
		var value string
		if err := json.Unmarshal(item, &value); err != nil {
			return nil, false, invalidStringArrayElementTypeError(toolName, field, idx, item)
		}
		values = append(values, value)
	}
	return values, true, nil
}

func decodeStringArrayItems(raw json.RawMessage) ([]json.RawMessage, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err == nil {
		return items, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || strings.EqualFold(trimmed, "null") {
		return nil, nil
	}
	if !strings.HasPrefix(trimmed, "[") {
		return []json.RawMessage{append(json.RawMessage(nil), raw...)}, nil
	}
	if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
		return nil, err
	}
	return items, nil
}

func invalidStringArrayTypeError(toolName, field string, raw json.RawMessage) error {
	got := readJSONType(raw)
	cause := fmt.Errorf("%s must be an array of strings; got %s", field, got)
	return &InvalidArgumentsError{
		ToolName: toolName,
		Message:  toolArgumentErrorMessage(toolName, false, cause),
		Cause:    cause,
	}
}

func invalidStringArrayElementTypeError(toolName, field string, index int, raw json.RawMessage) error {
	got := readJSONType(raw)
	cause := fmt.Errorf("%s[%d] must be a string; got %s", field, index, got)
	return &InvalidArgumentsError{
		ToolName: toolName,
		Message:  toolArgumentErrorMessage(toolName, false, cause),
		Cause:    cause,
	}
}

func NormalizeArgumentError(toolName string, err error) error {
	if err == nil {
		return nil
	}
	var invalid *InvalidArgumentsError
	if errors.As(err, &invalid) {
		return err
	}
	switch {
	case isMalformedArgumentError(err):
		return &InvalidArgumentsError{
			ToolName: toolName,
			Message:  toolArgumentErrorMessage(toolName, true, err),
			Cause:    err,
		}
	case isInvalidArgumentError(err):
		return &InvalidArgumentsError{
			ToolName: toolName,
			Message:  toolArgumentErrorMessage(toolName, false, err),
			Cause:    err,
		}
	default:
		return err
	}
}

func InvalidArguments(toolName string, cause error) error {
	if cause == nil {
		return nil
	}
	var invalid *InvalidArgumentsError
	if errors.As(cause, &invalid) {
		return cause
	}
	return &InvalidArgumentsError{
		ToolName: toolName,
		Message:  toolArgumentErrorMessage(toolName, false, cause),
		Cause:    cause,
	}
}

func DefaultErrorText(toolName string, err error) string {
	normalized := NormalizeArgumentError(toolName, err)
	if normalized == nil {
		return ""
	}
	var invalid *InvalidArgumentsError
	if errors.As(normalized, &invalid) {
		cause := invalid.Cause
		if cause == nil {
			cause = normalized
		}
		return strings.TrimSpace(toolArgumentErrorMessage(toolName, isMalformedArgumentError(cause), cause))
	}
	return strings.TrimSpace(normalized.Error())
}

func isMalformedArgumentError(err error) bool {
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return true
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	message := strings.TrimSpace(err.Error())
	return strings.Contains(message, "unexpected end of JSON input") ||
		strings.HasPrefix(message, "invalid character ")
}

func isInvalidArgumentError(err error) bool {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return true
	}
	message := strings.TrimSpace(err.Error())
	return strings.Contains(message, "cannot unmarshal")
}

func quotedToolName(toolName string) string {
	name := strings.TrimSpace(toolName)
	if name == "" {
		return "this tool"
	}
	return fmt.Sprintf("`%s`", name)
}

var malformedJSONBareValuePattern = regexp.MustCompile(`"([^"]+)"\s*:\s*([^"\[{0-9-][^,}\]]*)`)

func toolArgumentErrorMessage(toolName string, malformed bool, cause error) string {
	if strings.TrimSpace(toolName) == ApplyPatchToolName {
		return applyPatchArgumentErrorMessage(cause)
	}
	detail := strings.TrimSpace(errorDetailText(cause))
	if detail == "" {
		detail = "unknown error"
	}
	if malformed {
		detail = strings.TrimSpace(detail)
	}
	message := fmt.Sprintf("%s failed. %s.", quotedToolName(toolName), strings.TrimSuffix(detail, "."))
	if contract := toolArgumentContractText(toolName, cause); contract != "" {
		message += " " + contract
	}
	return message
}

func errorDetailText(err error) string {
	if err == nil {
		return ""
	}
	var invalid *InvalidArgumentsError
	if errors.As(err, &invalid) && invalid.Cause != nil {
		return errorDetailText(invalid.Cause)
	}
	var contextual *contextualArgumentDetailError
	if errors.As(err, &contextual) && strings.TrimSpace(contextual.Detail) != "" {
		return strings.TrimSpace(contextual.Detail)
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		if detail := unmarshalTypeErrorDetail(typeErr); detail != "" {
			return detail
		}
	}
	return err.Error()
}

func unmarshalTypeErrorDetail(err *json.UnmarshalTypeError) string {
	if err == nil {
		return ""
	}
	field := strings.TrimSpace(err.Field)
	if field == "" {
		field = "value"
	}
	switch err.Type.Kind() {
	case reflect.Bool:
		return field + " must be a boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return field + " must be an integer"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return field + " must be an integer"
	case reflect.String:
		return field + " must be a string"
	case reflect.Slice, reflect.Array:
		return field + " must be an array"
	case reflect.Map, reflect.Struct:
		return field + " must be an object"
	default:
		return ""
	}
}

func normalizeToolInputError(toolName string, err error) error {
	if err == nil {
		return nil
	}
	return InvalidArguments(toolName, err)
}

func withArgumentDetail(args json.RawMessage, err error) error {
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(argumentErrorDetailFromInput(args, err))
	if detail == "" || strings.EqualFold(detail, strings.TrimSpace(err.Error())) {
		return err
	}
	return &contextualArgumentDetailError{
		Detail: detail,
		Cause:  err,
	}
}

func argumentErrorDetailFromInput(args json.RawMessage, err error) string {
	if err == nil {
		return ""
	}
	if isMalformedArgumentError(err) {
		if detail := malformedArgumentDetail(args); detail != "" {
			return detail
		}
		message := strings.TrimSpace(err.Error())
		switch {
		case errors.Is(err, io.ErrUnexpectedEOF), strings.Contains(message, "unexpected end of JSON input"):
			return "JSON ended before the object was complete"
		}
	}
	return err.Error()
}

func malformedArgumentDetail(args json.RawMessage) string {
	match := malformedJSONBareValuePattern.FindStringSubmatch(strings.TrimSpace(string(args)))
	if len(match) != 3 {
		return ""
	}
	field := strings.TrimSpace(match[1])
	value := strings.TrimSpace(match[2])
	if field == "" || value == "" {
		return ""
	}
	switch strings.TrimSpace(value) {
	case "true", "false", "null":
		return ""
	}
	return fmt.Sprintf("%q has an unquoted string value; wrap it in double quotes", field)
}

func toolArgumentContractText(toolName string, cause error) string {
	switch strings.TrimSpace(toolName) {
	case ReadToolName:
		return `Use either path for one file or paths for one or more files; do not send both.`
	case "git_status":
		return "Use an empty object {} or omit arguments entirely."
	case TaskReviewToolName:
		return `Use action "list" or "review"; review requires task_id, review_status, and review_summary.`
	case TaskWorkflowToolName:
		return `Use action "list", "create", "update", "block", or "complete"; include the fields required by that action.`
	default:
		examples := toolArgumentExamples(toolName)
		if len(examples) == 0 {
			return ""
		}
		return "Example: " + examples[0] + "."
	}
}

func applyPatchArgumentErrorMessage(cause error) string {
	reason := applyPatchArgumentReason(cause)
	if reason == "" {
		reason = strings.TrimSpace(errorDetailText(cause))
	}
	if reason == "" {
		reason = "invalid patch"
	}
	message := "apply_patch: patch input: " + strings.TrimSuffix(reason, ".") + "."
	if hint := applyPatchArgumentHint(cause); hint != "" {
		message += " " + hint
	}
	return message
}

func applyPatchArgumentReason(cause error) string {
	switch {
	case errors.Is(cause, ErrApplyPatchMissingEnd):
		return "missing *** End Patch"
	case errors.Is(cause, ErrApplyPatchMissingBegin):
		return "missing *** Begin Patch"
	case errors.Is(cause, ErrApplyPatchEmpty):
		return "empty patch"
	case errors.Is(cause, ErrApplyPatchNoOperations):
		return "no file operation"
	case errors.Is(cause, ErrApplyPatchEmptyAdd):
		return "Add File has no added lines"
	case errors.Is(cause, ErrApplyPatchEmptyUpdate):
		return "Update File has no hunk or move"
	case errors.Is(cause, ErrApplyPatchReadLinePrefixes):
		return "read output line numbers in patch lines"
	case errors.Is(cause, ErrApplyPatchUnknownHeader):
		return "unknown file operation"
	case errors.Is(cause, ErrApplyPatchMalformedLine):
		return strings.TrimSpace(errorDetailText(cause))
	default:
		return strings.TrimSpace(errorDetailText(cause))
	}
}

func applyPatchArgumentHint(cause error) string {
	switch {
	case errors.Is(cause, ErrApplyPatchMissingEnd):
		return `End the patch with "*** End Patch".`
	case errors.Is(cause, ErrApplyPatchMissingBegin):
		return `Start the patch with "*** Begin Patch".`
	case errors.Is(cause, ErrApplyPatchEmpty), errors.Is(cause, ErrApplyPatchNoOperations):
		return "Send one complete patch with one file operation."
	case errors.Is(cause, ErrApplyPatchEmptyAdd):
		return `Prefix each added line with "+".`
	case errors.Is(cause, ErrApplyPatchEmptyUpdate):
		return `Add hunk lines or "*** Move to: ...".`
	case errors.Is(cause, ErrApplyPatchReadLinePrefixes):
		return "Remove line numbers copied from read output."
	case errors.Is(cause, ErrApplyPatchUnknownHeader):
		return `Use "*** Add File:", "*** Update File:", or "*** Delete File:".`
	case errors.Is(cause, ErrApplyPatchMalformedLine):
		return "Fix the patch syntax and retry."
	default:
		return ""
	}
}

func toolArgumentExamples(toolName string) []string {
	definition, ok := runtimeToolDefinition(toolName)
	if !ok {
		return nil
	}
	examples := make([]string, 0, len(definition.ArgumentExamples))
	for _, example := range definition.ArgumentExamples {
		if trimmed := strings.TrimSpace(example); trimmed != "" {
			examples = append(examples, trimmed)
		}
	}
	return examples
}

func runtimeToolDefinition(toolName string) (Definition, bool) {
	name := strings.TrimSpace(toolName)
	if name == "" {
		return Definition{}, false
	}
	for _, tl := range AllBuiltInTools() {
		definition := tl.Definition()
		if strings.TrimSpace(definition.Name) == name {
			return definition, true
		}
	}
	return Definition{}, false
}
