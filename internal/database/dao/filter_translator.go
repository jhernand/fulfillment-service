/*
Copyright (c) 2025 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package dao

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"unicode"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/operators"
	"github.com/google/cel-go/common/types/ref"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// FilterTranslatorBuilder contains the data and logic needed to create a filter translator. Don't create instances of
// this type directly, use the NewTranslationBuilder function instead.
type FilterTranslatorBuilder[O proto.Message] struct {
	logger *slog.Logger
}

// FilterTranslator knows how to translate filter expressions into SQL where clauses.
type FilterTranslator[O proto.Message] struct {
	logger   *slog.Logger
	tsDesc   protoreflect.MessageDescriptor
	thisDesc protoreflect.MessageDescriptor
	celEnv   *cel.Env
}

// filterTranslatorResultKind is the type of the result inferred during the translation process.
type filterTranslatorResultKind int

const (
	filterTranslatorNullType filterTranslatorResultKind = iota
	filterTranslatorBooleanKind
	filterTranslatorNumericKind
	filterTranslatorTimeKind
	filterTranslatorStringKind
	filterTranslatorThisKind
	filterTranslatorMdKind
	filterTranslatorJsonKind
)

// String returns a string representation of the translator result type.
func (t filterTranslatorResultKind) String() string {
	switch t {
	case filterTranslatorNullType:
		return "null"
	case filterTranslatorBooleanKind:
		return "boolean"
	case filterTranslatorNumericKind:
		return "numeric"
	case filterTranslatorStringKind:
		return "string"
	case filterTranslatorTimeKind:
		return "time"
	case filterTranslatorThisKind:
		return "this"
	case filterTranslatorMdKind:
		return "metadata"
	case filterTranslatorJsonKind:
		return "json"
	default:
		return fmt.Sprintf("unknown:%d", t)
	}
}

// filterTranslatorResult is the intermediate result of the translation process.
type filterTranslatorResult struct {
	// sql is the SQL text.
	sql string

	// precedence is the precedence of the operator used at the top of the translation. This is used to decide if
	// it is necessary to put parenthesis arround the text to use it in larger translations.
	//
	// Note that this is the precedence of SQL operators, not of CEL operators.
	precedence int

	// kind is the type of the result.
	kind filterTranslatorResultKind

	// desc is the descriptor of the type of the result. Will only be set when the kind of the result is a protobuf
	// message.
	desc protoreflect.MessageDescriptor
}

// Precendes of operators in the SQL language.
const (
	filterTranslatorMaxPrecedence            = math.MaxInt
	filterTranslatorMultiplicativePrecedence = 8
	filterTranslatorAdditivePrecedence       = 7
	filterTranslatorIsPrecedence             = 6
	filterTranslatorOtherPrecedence          = 5
	filterTranslatorInPrecedence             = 4
	filterTranslatorComparisonPrecedence     = 3
	filterTranslatorNotPrecedence            = 2
	filterTranslatorAndPrecedence            = 1
	filterTranslatorOrPrecedence             = 0
)

// NewFilterTranslator creates a object that knows how to translate filter expressions into SQL where statements.
func NewFilterTranslator[O proto.Message]() *FilterTranslatorBuilder[O] {
	return &FilterTranslatorBuilder[O]{}
}

// SetLogger sets the logger that will be used by the translator. This is mandatory.
func (b *FilterTranslatorBuilder[O]) SetLogger(value *slog.Logger) *FilterTranslatorBuilder[O] {
	b.logger = value
	return b
}

// Build uses the data stored in the builder to create and configure a new filter translator.
func (b *FilterTranslatorBuilder[O]) Build() (result *FilterTranslator[O], err error) {
	// Check parameters:
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}

	// Get the descriptors of well known types:
	var tsTempl *timestamppb.Timestamp
	tsDesc := tsTempl.ProtoReflect().Descriptor()

	// Get the object descriptor:
	var thisTempl O
	thisDesc := thisTempl.ProtoReflect().Descriptor()

	// Create the CEN environment:
	celEnv, err := b.createCelEnv()
	if err != nil {
		err = fmt.Errorf("failed to create CEL environment")
		return
	}

	// Create and populate the object:
	result = &FilterTranslator[O]{
		logger:   b.logger,
		tsDesc:   tsDesc,
		thisDesc: thisDesc,
		celEnv:   celEnv,
	}
	return
}

func (b *FilterTranslatorBuilder[O]) createCelEnv() (result *cel.Env, err error) {
	var options []cel.EnvOption

	// Declare the object type:
	var thisTemplate O
	options = append(options, cel.Types(thisTemplate))

	// Declare the object variable:
	thisDesc := thisTemplate.ProtoReflect().Descriptor()
	thisType := cel.ObjectType(string(thisDesc.FullName()))
	options = append(options, cel.Variable("this", thisType))

	// Declare the current date:
	options = append(options, cel.Variable("now", cel.TimestampType))

	// Create the CEL environment:
	result, err = cel.NewEnv(options...)
	return
}

// Translate translate the given filter expression into a SQL where statement.
func (t *FilterTranslator[O]) Translate(ctx context.Context, filter string) (sql string, err error) {
	ast, issues := t.celEnv.Compile(filter)
	if issues != nil {
		err = issues.Err()
		if err != nil {
			return
		}
	}
	result, err := t.translate(ast.NativeRep().Expr())
	if err != nil {
		return
	}
	sql = result.sql
	return
}

func (t *FilterTranslator[O]) translate(expr ast.Expr) (result filterTranslatorResult, err error) {
	switch expr.Kind() {
	case ast.CallKind:
		result, err = t.translateCall(expr.AsCall())
	case ast.IdentKind:
		result, err = t.translateIdent(expr.AsIdent())
	case ast.LiteralKind:
		result, err = t.translateLiteral(expr.AsLiteral())
	case ast.SelectKind:
		result, err = t.translateSelectField(expr.AsSelect())
	default:
		err = fmt.Errorf("unsupported expression kind %d", expr.Kind())
		return
	}
	return
}

func (t *FilterTranslator[O]) translateCall(expr ast.CallExpr) (result filterTranslatorResult, err error) {
	funcName := expr.FunctionName()
	funcArgs := expr.Args()
	switch funcName {
	case operators.Add,
		operators.Subtract,
		operators.Multiply,
		operators.Divide,
		operators.Modulo,
		operators.Equals,
		operators.NotEquals,
		operators.Greater,
		operators.GreaterEquals,
		operators.Less,
		operators.LessEquals,
		operators.LogicalAnd,
		operators.LogicalOr:
		if len(funcArgs) != 2 {
			err = fmt.Errorf(
				"expected exactly two arguments for operator '%s' but got %d",
				funcName, len(funcArgs),
			)
			return
		}
		result, err = t.translateBinary(funcName, funcArgs[0], funcArgs[1])
	case operators.LogicalNot:
		result, err = t.translateNot(funcArgs[0])
	case operators.In:
		result, err = t.translateIn(funcArgs)
	case "contains":
		if len(funcArgs) != 1 {
			err = fmt.Errorf(
				"expected exactly one argument for function '%s' but got %d",
				funcName, len(funcArgs),
			)
			return
		}
		result, err = t.translateToLike(funcName, expr.Target(), funcArgs[0], "%", "%")
	case "startsWith":
		if len(funcArgs) != 1 {
			err = fmt.Errorf(
				"expected exactly one argument for function '%s' but got %d",
				funcName, len(funcArgs),
			)
			return
		}
		result, err = t.translateToLike(funcName, expr.Target(), funcArgs[0], "", "%")
	case "endsWith":
		if len(funcArgs) != 1 {
			err = fmt.Errorf(
				"expected exactly one argument for function '%s' but got %d",
				funcName, len(funcArgs),
			)
			return
		}
		result, err = t.translateToLike(funcName, expr.Target(), funcArgs[0], "%", "")
	default:
		err = fmt.Errorf("function '%s' isn't supported", funcName)
		return
	}
	return
}

func (t *FilterTranslator[O]) translateBinary(name string, left, right ast.Expr) (result filterTranslatorResult, err error) {
	var (
		operatorSql      string
		resultPrecedence int
		resultKind       filterTranslatorResultKind
	)
	leftTr, err := t.translate(left)
	if err != nil {
		return
	}
	rightTr, err := t.translate(right)
	if err != nil {
		return
	}
	switch name {
	case operators.Add:
		operatorSql = "+"
		resultPrecedence = filterTranslatorAdditivePrecedence
		resultKind = leftTr.kind
	case operators.Subtract:
		operatorSql = "-"
		resultPrecedence = filterTranslatorAdditivePrecedence
		resultKind = leftTr.kind
	case operators.Multiply:
		operatorSql = "*"
		resultPrecedence = filterTranslatorMultiplicativePrecedence
		resultKind = leftTr.kind
	case operators.Divide:
		operatorSql = "/"
		resultPrecedence = filterTranslatorMultiplicativePrecedence
		resultKind = leftTr.kind
	case operators.Modulo:
		operatorSql = "%"
		resultPrecedence = filterTranslatorMultiplicativePrecedence
		resultKind = leftTr.kind
	case operators.Equals:
		// If one of the sides is a null expression then we swap sides so that the null is always on the right,
		// as that way we can convert it to 'is null'.
		if leftTr.kind == filterTranslatorNullType {
			leftTr, rightTr = rightTr, leftTr
		}
		if rightTr.kind == filterTranslatorNullType {
			operatorSql = "is"
			resultPrecedence = filterTranslatorIsPrecedence
		} else {
			operatorSql = "="
			resultPrecedence = filterTranslatorComparisonPrecedence
		}
		resultKind = filterTranslatorBooleanKind
	case operators.NotEquals:
		// If one of the sides is a null expression then we swap sides so that the null is always on the right,
		// as that way we can convert it to 'is not null'.
		if leftTr.kind == filterTranslatorNullType {
			leftTr, rightTr = rightTr, leftTr
		}
		if rightTr.kind == filterTranslatorNullType {
			operatorSql = "is not"
			resultPrecedence = filterTranslatorIsPrecedence
		} else {
			operatorSql = "!="
			resultPrecedence = filterTranslatorComparisonPrecedence
		}
		resultKind = filterTranslatorBooleanKind
	case operators.Greater:
		operatorSql = ">"
		resultPrecedence = filterTranslatorComparisonPrecedence
		resultKind = filterTranslatorBooleanKind
	case operators.GreaterEquals:
		operatorSql = ">="
		resultPrecedence = filterTranslatorComparisonPrecedence
		resultKind = filterTranslatorBooleanKind
	case operators.Less:
		operatorSql = "<"
		resultPrecedence = filterTranslatorComparisonPrecedence
		resultKind = filterTranslatorBooleanKind
	case operators.LessEquals:
		operatorSql = "<="
		resultPrecedence = filterTranslatorComparisonPrecedence
		resultKind = filterTranslatorBooleanKind
	case operators.LogicalAnd:
		operatorSql = "and"
		resultPrecedence = filterTranslatorAndPrecedence
		resultKind = filterTranslatorBooleanKind
	case operators.LogicalOr:
		operatorSql = "or"
		resultPrecedence = filterTranslatorOrPrecedence
		resultKind = filterTranslatorBooleanKind
	default:
		err = fmt.Errorf("unsupported operator '%s'", name)
		return
	}
	var buffer bytes.Buffer
	if leftTr.precedence < resultPrecedence {
		buffer.WriteString("(")
		buffer.WriteString(leftTr.sql)
		buffer.WriteString(")")
	} else {
		buffer.WriteString(leftTr.sql)
	}
	buffer.WriteString(" ")
	buffer.WriteString(operatorSql)
	buffer.WriteString(" ")
	if rightTr.precedence < resultPrecedence {
		buffer.WriteString("(")
		buffer.WriteString(rightTr.sql)
		buffer.WriteString(")")
	} else {
		buffer.WriteString(rightTr.sql)
	}
	result.sql = buffer.String()
	result.precedence = resultPrecedence
	result.kind = resultKind
	return
}

func (t *FilterTranslator[O]) translateNot(value ast.Expr) (result filterTranslatorResult, err error) {
	valueTr, err := t.translate(value)
	if err != nil {
		return
	}
	var buffer bytes.Buffer
	buffer.WriteString("not ")
	if valueTr.precedence < filterTranslatorNotPrecedence {
		buffer.WriteString("(")
		buffer.WriteString(valueTr.sql)
		buffer.WriteString(")")
	} else {
		buffer.WriteString(valueTr.sql)
	}
	result.sql = buffer.String()
	result.precedence = filterTranslatorNotPrecedence
	result.kind = filterTranslatorBooleanKind
	return
}

func (t *FilterTranslator[O]) translateIdent(name string) (result filterTranslatorResult, err error) {
	switch name {
	case "this":
		result.sql = ""
		result.kind = filterTranslatorThisKind
	case "now":
		result.sql = "now()"
		result.kind = filterTranslatorTimeKind
	default:
		err = fmt.Errorf("unknown identifier '%s'", name)
		return
	}
	result.precedence = filterTranslatorMaxPrecedence
	return
}

func (t *FilterTranslator[O]) translateLiteral(value ref.Val) (result filterTranslatorResult, err error) {
	switch value := value.Value().(type) {
	case structpb.NullValue:
		result.sql = "null"
		result.kind = filterTranslatorNullType
	case bool:
		result.sql = fmt.Sprintf("%v", value)
		result.kind = filterTranslatorBooleanKind
	case int64:
		result.sql = fmt.Sprintf("%d", value)
		result.kind = filterTranslatorNumericKind
	case string:
		text, escaped := t.translateString(value, "")
		if escaped {
			result.sql = "e'" + text + "'"
		} else {
			result.sql = "'" + text + "'"
		}
		result.kind = filterTranslatorStringKind
	default:
		err = fmt.Errorf("unknown literal type '%T'", value)
	}
	result.precedence = filterTranslatorMaxPrecedence
	return
}

// translateString translates the given string. If special is not empty then it is interpreted as an addition set of
// special characters that need to be escaped. This is intended for the creation of patterns for the 'like' operator,
// where it is necessary to escape the '%' and '_' characters. It returns the translated text, and a flag indicating if
// that text contains escape sequences that require the 'e' prefix.
func (t *FilterTranslator[O]) translateString(value, special string) (text string, escaped bool) {
	var buffer bytes.Buffer
	buffer.Grow(len(value))
	for _, r := range value {
		if r == '\\' || strings.ContainsRune(special, r) {
			buffer.WriteRune('\\')
			buffer.WriteRune(r)
		} else if r == '\'' {
			buffer.WriteString("\\'")
			escaped = true
		} else if r == '\n' {
			buffer.WriteString("\\n")
			escaped = true
		} else if r == '\t' {
			buffer.WriteString("\\t")
			escaped = true
		} else if unicode.IsPrint(r) && r <= unicode.MaxASCII {
			buffer.WriteRune(r)
		} else if r < math.MaxUint16 {
			fmt.Fprintf(&buffer, "\\u%04x", r)
			escaped = true
		} else {
			fmt.Fprintf(&buffer, "\\U%08x", r)
			escaped = true
		}
	}
	text = buffer.String()
	return
}

func (t *FilterTranslator[O]) translateIn(args []ast.Expr) (result filterTranslatorResult, err error) {
	key := args[0]
	keyTr, err := t.translate(key)
	if err != nil {
		return
	}
	values := args[1].AsList().Elements()
	valueTrs := make([]filterTranslatorResult, len(values))
	for i, value := range values {
		if value.Kind() != ast.LiteralKind {
			err = fmt.Errorf("value %d isn't a literal", i)
			return
		}
		valueTrs[i], err = t.translate(value)
		if err != nil {
			return
		}
	}
	var buffer bytes.Buffer
	if keyTr.precedence < filterTranslatorInPrecedence {
		fmt.Fprintf(&buffer, "(%s)", keyTr.sql)
	} else {
		buffer.WriteString(keyTr.sql)
	}
	buffer.WriteString(" in (")
	for i, valueTr := range valueTrs {
		if i > 0 {
			buffer.WriteString(", ")
		}
		buffer.WriteString(valueTr.sql)
	}
	buffer.WriteString(")")
	result.sql = buffer.String()
	result.precedence = filterTranslatorInPrecedence
	return
}

func (t *FilterTranslator[O]) translateToLike(funcName string, target ast.Expr, pattern ast.Expr,
	patternPrefix, patternSuffix string) (result filterTranslatorResult,
	err error) {
	var buffer bytes.Buffer
	targetTr, err := t.translate(target)
	if err != nil {
		return
	}
	if targetTr.precedence < filterTranslatorInPrecedence {
		buffer.WriteString("(")
		buffer.WriteString(targetTr.sql)
		buffer.WriteString(")")
	} else {
		buffer.WriteString(targetTr.sql)
	}
	buffer.WriteString(" like ")
	if pattern.Kind() != ast.LiteralKind {
		err = fmt.Errorf("argument of the '%s' function must be a string literal", funcName)
		return
	}
	patternLiteral := pattern.AsLiteral()
	patternValue, ok := patternLiteral.Value().(string)
	if !ok {
		err = fmt.Errorf("argument of the '%s' function must be a string literal", funcName)
		return
	}
	patternText, patternEscaped := t.translateString(patternValue, "%_")
	if patternEscaped {
		buffer.WriteString("e")
	}
	buffer.WriteString("'")
	buffer.WriteString(patternPrefix)
	buffer.WriteString(patternText)
	buffer.WriteString(patternSuffix)
	buffer.WriteString("'")
	result.sql = buffer.String()
	result.kind = filterTranslatorBooleanKind
	result.precedence = filterTranslatorInPrecedence
	return
}

func (t *FilterTranslator[O]) translateSelectField(expr ast.SelectExpr) (result filterTranslatorResult, err error) {
	operandTr, err := t.translate(expr.Operand())
	if err != nil {
		return
	}
	fieldName := expr.FieldName()
	testOnly := expr.IsTestOnly()
	switch operandTr.kind {
	case filterTranslatorThisKind:
		result, err = t.translateSelectThisField(fieldName, testOnly)
	case filterTranslatorMdKind:
		result, err = t.translateSelectThisMdField(fieldName, testOnly)
	case filterTranslatorJsonKind:
		result, err = t.translateSelectJsonField(operandTr.sql, operandTr.desc, fieldName, testOnly)
	default:
		err = fmt.Errorf("select of field '%s' of kind '%s' isn't supported", fieldName, operandTr.kind)
		return
	}
	result.precedence = filterTranslatorMaxPrecedence
	return
}

func (t *FilterTranslator[O]) translateSelectThisField(fieldName string, testOnly bool) (result filterTranslatorResult,
	err error) {
	switch fieldName {
	case "id":
		if testOnly {
			result.sql = "true"
			result.kind = filterTranslatorBooleanKind
			result.precedence = filterTranslatorMaxPrecedence
		} else {
			result.sql = fieldName
			result.kind = filterTranslatorStringKind
			result.precedence = filterTranslatorMaxPrecedence
		}
	case "metadata":
		if testOnly {
			result.sql = "true"
			result.kind = filterTranslatorBooleanKind
			result.precedence = filterTranslatorMaxPrecedence
		} else {
			result.sql = ""
			result.kind = filterTranslatorMdKind
			result.precedence = filterTranslatorMaxPrecedence
		}
	default:
		result, err = t.translateSelectJsonField("public_data", t.thisDesc, fieldName, testOnly)
	}
	return
}

func (t *FilterTranslator[O]) translateSelectThisMdField(fieldName string,
	testOnly bool) (result filterTranslatorResult, err error) {
	switch fieldName {
	case "creation_timestamp":
		// Note that we don't need to worry about this being null, because it will never be.
		if testOnly {
			result.sql = "true"
			result.kind = filterTranslatorBooleanKind
			result.precedence = filterTranslatorMaxPrecedence
		} else {
			result.sql = fieldName
			result.kind = filterTranslatorTimeKind
			result.precedence = filterTranslatorMaxPrecedence
		}
	case "deletion_timestamp":
		// The deletion timestamp doesn't accept null values in the database, instead it is set to the Unix
		// epoch when there object hasn't been deleted, so we need to translate that into a null in order to be
		// able to compare to other things that may be null. For example the following filter expression:
		//
		//	this.metadata.deletion_timestamp != null
		//
		// Can't be translated into this, because the result will always be `true``:
		//
		//	deletion_timestamp is not null
		//
		// Instead we will translate into this:
		//
		//	nullif(deletion_timestamp, '1970-01-01 00:00:00Z') is not null
		//
		// That will return `false` if the date is set to the Unix epoch, and `true` if the date has any other
		// value.
		if testOnly {
			result.sql = fmt.Sprintf("%s != '1970-01-01 00:00:00Z'", fieldName)
			result.kind = filterTranslatorBooleanKind
			result.precedence = filterTranslatorMaxPrecedence
		} else {
			result.sql = fmt.Sprintf("nullif(%s, '1970-01-01 00:00:00Z')", fieldName)
			result.kind = filterTranslatorTimeKind
			result.precedence = filterTranslatorMaxPrecedence
		}
	default:
		err = fmt.Errorf("metadata doesn't have a '%s' field", fieldName)
	}
	return
}

func (t *FilterTranslator[O]) translateSelectJsonField(operandSql string, msgDesc protoreflect.MessageDescriptor,
	fieldName string, testOnly bool) (result filterTranslatorResult, err error) {
	if testOnly {
		result.sql = fmt.Sprintf("%s ? '%s'", operandSql, fieldName)
		result.kind = filterTranslatorBooleanKind
		result.precedence = filterTranslatorOtherPrecedence
		return
	}
	fieldDesc := msgDesc.Fields().ByName(protoreflect.Name(fieldName))
	fieldKind := fieldDesc.Kind()
	switch fieldKind {
	case protoreflect.BoolKind:
		result.sql = fmt.Sprintf("cast(%s->>'%s' as bool)", operandSql, fieldName)
		result.kind = filterTranslatorBooleanKind
	case protoreflect.Int32Kind:
		result.sql = fmt.Sprintf("cast(%s->>'%s' as integer)", operandSql, fieldName)
		result.kind = filterTranslatorNumericKind
	case protoreflect.Int64Kind:
		result.sql = fmt.Sprintf("cast(%s->>'%s' as bigint)", operandSql, fieldName)
		result.kind = filterTranslatorNumericKind
	case protoreflect.FloatKind:
		result.sql = fmt.Sprintf("cast(%s->>'%s' as real)", operandSql, fieldName)
		result.kind = filterTranslatorNumericKind
	case protoreflect.DoubleKind:
		result.sql = fmt.Sprintf("cast(%s->>'%s' as double precision)", operandSql, fieldName)
		result.kind = filterTranslatorNumericKind
	case protoreflect.StringKind:
		result.sql = fmt.Sprintf("%s->>'%s'", operandSql, fieldName)
		result.kind = filterTranslatorStringKind
	case protoreflect.MessageKind:
		msgDesc := fieldDesc.Message()
		switch msgDesc {
		case t.tsDesc:
			result.sql = fmt.Sprintf("cast(%s->>'%s' as timestamp with time zone)", operandSql, fieldName)
			result.kind = filterTranslatorTimeKind
		default:
			result.sql = fmt.Sprintf("%s->'%s'", operandSql, fieldName)
			result.kind = filterTranslatorJsonKind
			result.desc = fieldDesc.Message()
		}
	default:
		err = fmt.Errorf(
			"select of JSON field '%s' of operand '%s' of type '%s' of kind '%s' isn't supported",
			fieldName, operandSql, msgDesc.FullName(), fieldKind,
		)
		return
	}
	result.precedence = filterTranslatorMaxPrecedence
	return
}
