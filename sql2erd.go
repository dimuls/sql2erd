package sql2erd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/auxten/postgresql-parser/pkg/sql/parser"
	"github.com/auxten/postgresql-parser/pkg/sql/sem/tree"
	"github.com/auxten/postgresql-parser/pkg/walk"
	"github.com/davecgh/go-spew/spew"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2layouts/d2elklayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2oracle"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

type Renderer struct {
	In  io.Reader
	Out io.Writer
}

type column struct {
	name       string
	dataType   string
	primary    bool
	notNull    bool
	unique     bool
	references bool
}

type reference struct {
	fromTable   string
	toTable     string
	fromColumns []string
	toColumns   []string
}

type table struct {
	name    string
	columns []*column
}

var (
	filters = []string{
		"owner to",
		"create extension",
		"create or replace extension",
		"comment on extension",
		"create sequence",
		"create or replace sequence",
		"create trigger",
		"create or replace trigger",
	}
)

func (r Renderer) Render(ctx context.Context) error {
	in, err := io.ReadAll(r.In)
	if err != nil {
		return fmt.Errorf("read in: %w", err)
	}

	const statementSeparator = ";"

	var filtersRegexps []*regexp.Regexp
	for _, f := range filters {
		filtersRegexps = append(filtersRegexps, regexp.MustCompile(f))
	}

	var filtered []string

loop:
	for _, s := range strings.Split(string(in), statementSeparator) {
		s = strings.ToLower(s)
		for _, f := range filtersRegexps {
			if f.MatchString(s) {
				continue loop
			}
		}
		filtered = append(filtered, s)
	}

	statements, err := parser.Parse(strings.Join(filtered, statementSeparator))
	if err != nil {
		return fmt.Errorf("parse sql: %w", err)
	}

	var (
		tables            []table
		columns           = map[string]*column{}
		complexUniqueKeys = map[string]bool{}
		references        []reference
		t                 *table
		walkErr           error
	)

	spew.Config.DisableMethods = true

	w := &walk.AstWalker{
		Fn: func(_ interface{}, node interface{}) (stop bool) {
			switch n := node.(type) {
			case *tree.CreateTable:
				if t != nil {
					tables = append(tables, *t)
				}
				t = &table{
					name: n.Table.TableName.Normalize(),
				}
			case *tree.ColumnTableDef:
				name := n.Name.Normalize()

				c := &column{
					name:     n.Name.Normalize(),
					dataType: n.Type.SQLString(),
					primary:  n.PrimaryKey.IsPrimaryKey,
					notNull:  n.Nullable.Nullability == tree.NotNull,
					unique:   n.Unique,
				}

				t.columns = append(t.columns, c)
				columns[t.name+"."+name] = c

				if n.References.Table != nil {
					c.references = true
					references = append(references, reference{
						fromTable:   t.name,
						toTable:     n.References.Table.TableName.Normalize(),
						fromColumns: []string{name},
						toColumns:   []string{n.References.Col.Normalize()},
					})
				}

			case *tree.ForeignKeyConstraintTableDef:
				var fromColumns, toColumns []string
				for _, c := range n.FromCols {
					fromColumns = append(fromColumns, c.Normalize())
				}

				unsetToColumns := true
				for i, c := range n.ToCols {
					toColumn := c.Normalize()

					if fromColumns[i] != toColumn {
						unsetToColumns = false
					}

					toColumns = append(toColumns, toColumn)
				}

				if unsetToColumns {
					toColumns = nil
				}

				references = append(references, reference{
					fromTable:   t.name,
					toTable:     n.Table.TableName.Normalize(),
					fromColumns: fromColumns,
					toColumns:   toColumns,
				})
			case *tree.UniqueConstraintTableDef:
				for _, c := range n.Columns {
					columnName := c.Column.Normalize()

					columns[t.name+"."+columnName].unique = true
					columns[t.name+"."+columnName].primary = n.PrimaryKey

					if len(n.Columns) > 1 {
						complexUniqueKeys[t.name+"."+columnName] = true
					}
				}

			case *tree.AlterTable:
				if t != nil {
					tables = append(tables, *t)
					t = nil
				}

				tableName := n.Table.ToTableName().TableName.Normalize()

				for _, tt := range tables {
					if tt.name == tableName {
						t = &tt
						break
					}
				}

				if t == nil {
					walkErr = fmt.Errorf("expected altering table %q exists", tableName)

					return true
				}

				for _, cmd := range n.Cmds {
					switch cmdt := cmd.(type) {
					case *tree.AlterTableAddConstraint:
						switch constraint := cmdt.ConstraintDef.(type) {
						case *tree.UniqueConstraintTableDef:
							for _, c := range constraint.Columns {
								columnName := c.Column.Normalize()

								columns[t.name+"."+columnName].unique = true
								columns[t.name+"."+columnName].primary = constraint.PrimaryKey

								if len(constraint.Columns) > 1 {
									complexUniqueKeys[t.name+"."+columnName] = true
								}
							}
						case *tree.ForeignKeyConstraintTableDef:
							var fromColumns, toColumns []string
							for _, c := range constraint.FromCols {
								fromColumns = append(fromColumns, c.Normalize())
							}

							unsetToColumns := true
							for i, c := range constraint.ToCols {
								toColumn := c.Normalize()

								if fromColumns[i] != toColumn {
									unsetToColumns = false
								}

								toColumns = append(toColumns, toColumn)
							}

							if unsetToColumns {
								toColumns = nil
							}

							references = append(references, reference{
								fromTable:   t.name,
								toTable:     constraint.Table.TableName.Normalize(),
								fromColumns: fromColumns,
								toColumns:   toColumns,
							})
						}
					}
				}

				t = nil
			}
			return false
		},
	}

	_, err = w.Walk(statements, nil)
	if err != nil {
		return fmt.Errorf("walk sql: %w", err)
	}
	if walkErr != nil {
		return fmt.Errorf("walk sql: %w", walkErr)
	}

	if t != nil {
		tables = append(tables, *t)
	}

	_, graph, err := d2lib.Compile(ctx, "", nil)
	if err != nil {
		return fmt.Errorf("d2lib compile: %w", err)
	}

	for _, t := range tables {
		var key string

		graph, key, _ = d2oracle.Create(graph, t.name)
		shape := "sql_table"
		graph, err = d2oracle.Set(graph, fmt.Sprintf("%s.shape", key), nil, &shape)
		if err != nil {
			return fmt.Errorf("create table on diagram: %w", err)
		}

		for _, c := range t.columns {
			var constraints []string
			if c.notNull && !c.primary {
				constraints = append(constraints, "NOTNULL")
			}
			if c.unique && !c.primary {
				constraints = append(constraints, "UNQ")
			}
			if c.primary {
				constraints = append(constraints, "PK")
			}
			if c.references {
				constraints = append(constraints, "FK")
			}

			graph, err = d2oracle.Set(graph, fmt.Sprintf("%s.%s", t.name, c.name), nil, &c.dataType)
			if err != nil {
				return fmt.Errorf("create table column on diagram: %w", err)
			}

			constraint := strings.Join(constraints, ", ")

			graph, err = d2oracle.Set(graph, fmt.Sprintf("%s.%s.constraint", t.name, c.name), nil, &constraint)
			if err != nil {
				return fmt.Errorf("set table column on diagram: %w", err)
			}
		}
	}

	for _, r := range references {
		var key string

		graph, key, err = d2oracle.Create(graph, fmt.Sprintf("%s <-> %s", r.fromTable, r.toTable))
		if err != nil {
			return fmt.Errorf("create reference on diagram: %w", err)
		}

		var name string

		if len(r.toColumns) == 0 {
			name = fmt.Sprintf("%s", strings.Join(r.fromColumns, ", "))
		} else {
			name = fmt.Sprintf("%s: %s", strings.Join(r.fromColumns, ", "), strings.Join(r.toColumns, ", "))
		}

		graph, err = d2oracle.Set(graph, key, nil, &name)
		if err != nil {
			return fmt.Errorf("name reference on diagram: %w", err)
		}

		var (
			cfOne          = string(d2target.CfOne)
			cfOneRequired  = string(d2target.CfOneRequired)
			cfMany         = string(d2target.CfMany)
			cfManyRequired = string(d2target.CfManyRequired)

			value *string

			fromNotNull = true
			toNotNull   = true

			fromUnique = true
			toUnique   = true
		)

		for _, c := range r.fromColumns {
			columnKey := r.fromTable + "." + c
			fromNotNull = fromNotNull && (columns[columnKey].primary || columns[r.fromTable+"."+c].notNull)
			fromUnique = fromUnique && (columns[columnKey].primary || columns[columnKey].unique) && !complexUniqueKeys[columnKey]
		}

		if fromNotNull {
			if fromUnique {
				value = &cfOneRequired
			} else {
				value = &cfManyRequired
			}
		} else {
			if fromUnique {
				value = &cfOne
			} else {
				value = &cfMany
			}
		}
		graph, err = d2oracle.Set(graph, fmt.Sprintf("%s.source-arrowhead.shape", key), nil, value)
		if err != nil {
			return fmt.Errorf("style reference on diagram: %w", err)
		}

		for _, c := range r.toColumns {
			columnKey := r.toTable + "." + c
			toNotNull = toNotNull && (columns[columnKey].primary || columns[columnKey].notNull)
			toUnique = toUnique && (columns[columnKey].primary || columns[columnKey].unique) && !complexUniqueKeys[columnKey]
		}

		if toNotNull {
			if toUnique {
				value = &cfOneRequired
			} else {
				value = &cfManyRequired
			}
		} else {
			if toUnique {
				value = &cfOne
			} else {
				value = &cfMany
			}
		}
		graph, err = d2oracle.Set(graph, fmt.Sprintf("%s.target-arrowhead.shape", key), nil, value)
		if err != nil {
			return fmt.Errorf("style reference on diagram: %w", err)
		}
	}

	script := d2format.Format(graph.AST)

	ruler, _ := textmeasure.NewRuler()

	diagram, _, err := d2lib.Compile(context.Background(), script, &d2lib.CompileOptions{
		Layout: d2elklayout.DefaultLayout,
		Ruler:  ruler,
	})
	if err != nil {
		return fmt.Errorf("create diagram: %w", err)
	}

	out, err := d2svg.Render(diagram, &d2svg.RenderOpts{
		Pad: d2svg.DEFAULT_PADDING,
	})
	if err != nil {
		return fmt.Errorf("d2svg render: %w", err)
	}

	_, err = io.Copy(r.Out, bytes.NewReader(out))
	if err != nil {
		return fmt.Errorf("write svg to out: %w", err)
	}

	return nil
}
