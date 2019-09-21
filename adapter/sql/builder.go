package sql

import (
	"bytes"
	"strconv"
	"strings"
	"sync"

	"github.com/Fs02/grimoire"
)

// UnescapeCharacter disable field escaping when it starts with this character.
var UnescapeCharacter byte = '^'

var fieldCache sync.Map

// Builder defines information of query b.
type Builder struct {
	config      *Config
	returnField string
	count       int
}

// Find generates query for select.
func (b *Builder) Find(query grimoire.Query) (string, []interface{}) {
	var (
		buffer bytes.Buffer
	)

	b.fields(&buffer, query.SelectQuery.OnlyDistinct, query.SelectQuery.Fields)
	args := b.query(&buffer, query)

	return buffer.String(), args
}

// Aggregate generates query for aggregation.
func (b *Builder) Aggregate(query grimoire.Query, mode string, field string) (string, []interface{}) {
	var (
		buffer      bytes.Buffer
		selectfield = mode + "(" + field + ") AS " + mode
	)

	b.fields(&buffer, false, append(query.GroupQuery.Fields, selectfield))
	args := b.query(&buffer, query)

	return buffer.String(), args
}

func (b *Builder) query(buffer *bytes.Buffer, query grimoire.Query) []interface{} {
	var (
		args []interface{}
	)

	b.from(buffer, query.Collection)

	if arg := b.join(buffer, query.JoinQuery...); arg != nil {
		args = append(args, arg...)
	}

	if !query.WhereQuery.None() {
		arg := b.where(buffer, query.WhereQuery)
		args = append(args, arg...)
	}

	if len(query.GroupQuery.Fields) > 0 {
		b.groupBy(buffer, query.GroupQuery.Fields)

		if !query.GroupQuery.Filter.None() {
			arg := b.having(buffer, query.GroupQuery.Filter)
			args = append(args, arg...)
		}
	}

	if len(query.SortQuery) > 0 {
		b.orderBy(buffer, query.SortQuery)
	}

	b.limitOffset(buffer, query.LimitQuery, query.OffsetQuery)

	if query.LockQuery != "" {
		buffer.WriteString(" ")
		buffer.WriteString(string(query.LockQuery))
	}

	buffer.WriteString(";")

	return args
}

// Insert generates query for insert.
func (b *Builder) Insert(collection string, changes grimoire.Changes) (string, []interface{}) {
	var (
		buffer bytes.Buffer
		length = len(changes.Changes)
		args   = make([]interface{}, 0, length)
	)

	buffer.WriteString("INSERT INTO ")
	buffer.WriteString(b.escape(collection))

	if length == 0 && b.config.InsertDefaultValues {
		buffer.WriteString(" DEFAULT VALUES")
	} else {
		buffer.WriteString(" (")

		for i, ch := range changes.Changes {
			switch ch.Type {
			case grimoire.ChangeSetOp:
				buffer.WriteString(b.config.EscapeChar)
				buffer.WriteString(ch.Field)
				buffer.WriteString(b.config.EscapeChar)
				args = append(args, ch.Value)
			case grimoire.ChangeFragmentOp:
				buffer.WriteString(ch.Field)
				args = append(args, ch.Value.([]interface{})...)
			case grimoire.ChangeIncOp, grimoire.ChangeDecOp:
				continue
			}

			if i < length-1 {
				buffer.WriteString(",")
			}
		}

		buffer.WriteString(") VALUES ")

		buffer.WriteString("(")
		for i := 0; i < len(args); i++ {
			buffer.WriteString(b.ph())

			if i < len(args)-1 {
				buffer.WriteString(",")
			}
		}
		buffer.WriteString(")")
	}

	if b.returnField != "" {
		buffer.WriteString(" RETURNING ")
		buffer.WriteString(b.config.EscapeChar)
		buffer.WriteString(b.returnField)
		buffer.WriteString(b.config.EscapeChar)
	}

	buffer.WriteString(";")

	return buffer.String(), args
}

// InsertAll generates query for multiple insert.
func (b *Builder) InsertAll(collection string, fields []string, allchanges []grimoire.Changes) (string, []interface{}) {
	var (
		buffer bytes.Buffer
		args   = make([]interface{}, 0, len(fields)*len(allchanges))
	)

	buffer.WriteString("INSERT INTO ")

	buffer.WriteString(b.config.EscapeChar)
	buffer.WriteString(collection)
	buffer.WriteString(b.config.EscapeChar)

	sep := b.config.EscapeChar + "," + b.config.EscapeChar

	buffer.WriteString(" (")
	buffer.WriteString(b.config.EscapeChar)
	buffer.WriteString(strings.Join(fields, sep))
	buffer.WriteString(b.config.EscapeChar)
	buffer.WriteString(") VALUES ")

	for i, changes := range allchanges {
		buffer.WriteString("(")

		for j, field := range fields {
			if ch, ok := changes.Get(field); ok && ch.Type == grimoire.ChangeSetOp {
				buffer.WriteString(b.ph())
				args = append(args, ch.Value)
			} else {
				buffer.WriteString("DEFAULT")
			}

			if j < len(fields)-1 {
				buffer.WriteString(",")
			}
		}

		if i < len(allchanges)-1 {
			buffer.WriteString("),")
		} else {
			buffer.WriteString(")")
		}
	}

	if b.returnField != "" {
		buffer.WriteString(" RETURNING ")
		buffer.WriteString(b.config.EscapeChar)
		buffer.WriteString(b.returnField)
		buffer.WriteString(b.config.EscapeChar)
	}

	buffer.WriteString(";")

	return buffer.String(), args
}

// Update generates query for update.
func (b *Builder) Update(collection string, changes grimoire.Changes, filter grimoire.FilterQuery) (string, []interface{}) {
	var (
		buffer bytes.Buffer
		length = len(changes.Changes)
		args   = make([]interface{}, 0, length)
	)

	buffer.WriteString("UPDATE ")
	buffer.WriteString(b.config.EscapeChar)
	buffer.WriteString(collection)
	buffer.WriteString(b.config.EscapeChar)
	buffer.WriteString(" SET ")

	for i, ch := range changes.Changes {
		switch ch.Type {
		case grimoire.ChangeSetOp:
			buffer.WriteString(b.escape(ch.Field))
			buffer.WriteString("=")
			buffer.WriteString(b.ph())
			args = append(args, ch.Value)
		case grimoire.ChangeIncOp:
			buffer.WriteString(b.escape(ch.Field))
			buffer.WriteString("=")
			buffer.WriteString(b.escape(ch.Field))
			buffer.WriteString("+")
			buffer.WriteString(b.ph())
			args = append(args, ch.Value)
		case grimoire.ChangeDecOp:
			buffer.WriteString(b.escape(ch.Field))
			buffer.WriteString("=")
			buffer.WriteString(b.escape(ch.Field))
			buffer.WriteString("-")
			buffer.WriteString(b.ph())
			args = append(args, ch.Value)
		case grimoire.ChangeFragmentOp:
			buffer.WriteString(ch.Field)
			args = append(args, ch.Value.([]interface{})...)
		}

		if i < length-1 {
			buffer.WriteString(",")
		}
	}

	if !filter.None() {
		arg := b.where(&buffer, filter)
		args = append(args, arg...)
	}

	buffer.WriteString(";")

	return buffer.String(), args
}

// Delete generates query for delete.
func (b *Builder) Delete(collection string, filter grimoire.FilterQuery) (string, []interface{}) {
	var (
		buffer bytes.Buffer
		args   []interface{}
	)

	buffer.WriteString("DELETE FROM ")
	buffer.WriteString(b.config.EscapeChar)
	buffer.WriteString(collection)
	buffer.WriteString(b.config.EscapeChar)

	if !filter.None() {
		arg := b.where(&buffer, filter)
		args = append(args, arg...)
	}

	buffer.WriteString(";")

	return buffer.String(), args
}

func (b *Builder) fields(buffer *bytes.Buffer, distinct bool, fields []string) {
	if len(fields) == 0 {
		if distinct {
			buffer.WriteString("SELECT DISTINCT *")
			return
		}
		buffer.WriteString("SELECT *")
		return
	}

	buffer.WriteString("SELECT ")

	if distinct {
		buffer.WriteString("DISTINCT ")
	}

	l := len(fields) - 1
	for i, f := range fields {
		buffer.WriteString(b.escape(f))

		if i < l {
			buffer.WriteString(",")
		}
	}
}

func (b *Builder) from(buffer *bytes.Buffer, collection string) {
	buffer.WriteString(" FROM ")
	buffer.WriteString(b.config.EscapeChar)
	buffer.WriteString(collection)
	buffer.WriteString(b.config.EscapeChar)
}

func (b *Builder) join(buffer *bytes.Buffer, joins ...grimoire.JoinQuery) []interface{} {
	if len(joins) == 0 {
		return nil
	}

	var (
		args []interface{}
	)

	for _, join := range joins {
		buffer.WriteString(" ")
		buffer.WriteString(join.Mode)
		buffer.WriteString(" ")
		buffer.WriteString(b.config.EscapeChar)
		buffer.WriteString(join.Collection)
		buffer.WriteString(b.config.EscapeChar)
		buffer.WriteString(" ON ")
		buffer.WriteString(b.escape(join.From))
		buffer.WriteString("=")
		buffer.WriteString(b.escape(join.To))

		args = append(args, join.Arguments...)
	}

	return args
}

func (b *Builder) where(buffer *bytes.Buffer, filter grimoire.FilterQuery) []interface{} {
	buffer.WriteString(" WHERE ")
	return b.filter(buffer, filter)
}

func (b *Builder) groupBy(buffer *bytes.Buffer, fields []string) {
	buffer.WriteString(" GROUP BY ")

	l := len(fields) - 1
	for i, f := range fields {
		buffer.WriteString(b.escape(f))

		if i < l {
			buffer.WriteString(",")
		}
	}
}

func (b *Builder) having(buffer *bytes.Buffer, filter grimoire.FilterQuery) []interface{} {
	buffer.WriteString(" HAVING ")
	return b.filter(buffer, filter)
}

func (b *Builder) orderBy(buffer *bytes.Buffer, orders []grimoire.SortQuery) {
	l := len(orders)

	buffer.WriteString(" ORDER BY")
	for i, order := range orders {
		buffer.WriteByte(' ')
		buffer.WriteString(b.escape(order.Field))

		if order.Asc() {
			buffer.WriteString(" ASC")
		} else {
			buffer.WriteString(" DESC")
		}

		if i < l-1 {
			buffer.WriteByte(',')
		}
	}
}

func (b *Builder) limitOffset(buffer *bytes.Buffer, limit grimoire.Limit, offset grimoire.Offset) {
	if limit > 0 {
		buffer.WriteString(" LIMIT ")
		buffer.WriteString(strconv.Itoa(int(limit)))

		if offset > 0 {
			buffer.WriteString(" OFFSET ")
			buffer.WriteString(strconv.Itoa(int(offset)))
		}
	}
}

func (b *Builder) filter(buffer *bytes.Buffer, filter grimoire.FilterQuery) []interface{} {
	var (
		args []interface{}
	)

	switch filter.Type {
	case grimoire.FilterAndOp:
		args = b.build(buffer, "AND", filter.Inner)
	case grimoire.FilterOrOp:
		args = b.build(buffer, "OR", filter.Inner)
	case grimoire.FilterNotOp:
		buffer.WriteString("NOT ")
		args = b.build(buffer, "AND", filter.Inner)
	case grimoire.FilterEqOp,
		grimoire.FilterNeOp,
		grimoire.FilterLtOp,
		grimoire.FilterLteOp,
		grimoire.FilterGtOp,
		grimoire.FilterGteOp:
		args = b.buildComparison(buffer, filter)
	case grimoire.FilterNilOp:
		buffer.WriteString(b.escape(filter.Field))
		buffer.WriteString(" IS NULL")
		args = filter.Values
	case grimoire.FilterNotNilOp:
		buffer.WriteString(b.escape(filter.Field))
		buffer.WriteString(" IS NOT NULL")
		args = filter.Values
	case grimoire.FilterInOp,
		grimoire.FilterNinOp:
		args = b.buildInclusion(buffer, filter)
	case grimoire.FilterLikeOp:
		buffer.WriteString(b.escape(filter.Field))
		buffer.WriteString(" LIKE ")
		buffer.WriteString(b.ph())
		args = filter.Values
	case grimoire.FilterNotLikeOp:
		buffer.WriteString(b.escape(filter.Field))
		buffer.WriteString(" NOT LIKE ")
		buffer.WriteString(b.ph())
		args = filter.Values
	case grimoire.FilterFragmentOp:
		buffer.WriteString(filter.Field)
		args = filter.Values
	}

	return args
}

func (b *Builder) build(buffer *bytes.Buffer, op string, inner []grimoire.FilterQuery) []interface{} {
	var (
		length = len(inner)
		args   []interface{}
	)

	if length > 1 {
		buffer.WriteByte('(')
	}

	for i, c := range inner {
		cArgs := b.filter(buffer, c)
		args = append(args, cArgs...)

		if i < length-1 {
			buffer.WriteByte(' ')
			buffer.WriteString(op)
			buffer.WriteByte(' ')
		}
	}

	if length > 1 {
		buffer.WriteByte(')')
	}

	return args
}

func (b *Builder) buildComparison(buffer *bytes.Buffer, filter grimoire.FilterQuery) []interface{} {
	buffer.WriteString(b.escape(filter.Field))

	switch filter.Type {
	case grimoire.FilterEqOp:
		buffer.WriteByte('=')
	case grimoire.FilterNeOp:
		buffer.WriteString("<>")
	case grimoire.FilterLtOp:
		buffer.WriteByte('<')
	case grimoire.FilterLteOp:
		buffer.WriteString("<=")
	case grimoire.FilterGtOp:
		buffer.WriteByte('>')
	case grimoire.FilterGteOp:
		buffer.WriteString(">=")
	}

	buffer.WriteString(b.ph())

	return filter.Values
}

func (b *Builder) buildInclusion(buffer *bytes.Buffer, filter grimoire.FilterQuery) []interface{} {
	buffer.WriteString(b.escape(filter.Field))

	if filter.Type == grimoire.FilterInOp {
		buffer.WriteString(" IN (")
	} else {
		buffer.WriteString(" NOT IN (")
	}

	buffer.WriteString(b.ph())
	for i := 1; i <= len(filter.Values)-1; i++ {
		buffer.WriteString(",")
		buffer.WriteString(b.ph())
	}
	buffer.WriteString(")")

	return filter.Values
}

func (b *Builder) ph() string {
	if b.config.Ordinal {
		b.count++
		return b.config.Placeholder + strconv.Itoa(b.count)
	}

	return b.config.Placeholder
}

type fieldCacheKey struct {
	field  string
	escape string
}

func (b *Builder) escape(field string) string {
	if b.config.EscapeChar == "" || field == "*" {
		return field
	}

	key := fieldCacheKey{field: field, escape: b.config.EscapeChar}
	escapedField, ok := fieldCache.Load(key)
	if ok {
		return escapedField.(string)
	}

	if len(field) > 0 && field[0] == UnescapeCharacter {
		escapedField = field[1:]
	} else if start, end := strings.IndexRune(field, '('), strings.IndexRune(field, ')'); start >= 0 && end >= 0 && end > start {
		escapedField = field[:start+1] + b.escape(field[start+1:end]) + field[end:]
	} else if strings.HasSuffix(field, "*") {
		escapedField = b.config.EscapeChar + strings.Replace(field, ".", b.config.EscapeChar+".", 1)
	} else {
		escapedField = b.config.EscapeChar +
			strings.Replace(field, ".", b.config.EscapeChar+"."+b.config.EscapeChar, 1) +
			b.config.EscapeChar
	}

	fieldCache.Store(key, escapedField)
	return escapedField.(string)
}

// Returning append returning to insert grimoire.
func (b *Builder) Returning(field string) *Builder {
	b.returnField = field
	return b
}

// NewBuilder create new SQL builder.
func NewBuilder(config *Config) *Builder {
	return &Builder{
		config: config,
	}
}
