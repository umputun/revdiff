package highlight

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
)

func init() {
	styles.Register(chroma.MustNewStyle("one-light", chroma.StyleEntries{
		chroma.Background:          "#383a42 bg:#fafafa",
		chroma.Keyword:             "#a626a4",
		chroma.KeywordConstant:     "#0184bc",
		chroma.KeywordDeclaration:  "#a626a4",
		chroma.KeywordType:         "#c18401",
		chroma.Name:                "#383a42",
		chroma.NameAttribute:       "#c18401",
		chroma.NameBuiltin:         "#0184bc",
		chroma.NameClass:           "#c18401",
		chroma.NameConstant:        "#986801",
		chroma.NameDecorator:       "#4078f2",
		chroma.NameFunction:        "#4078f2",
		chroma.NameTag:             "#e45649",
		chroma.NameVariable:        "#e45649",
		chroma.Literal:             "#986801",
		chroma.LiteralString:       "#50a14f",
		chroma.LiteralStringChar:   "#50a14f",
		chroma.LiteralStringEscape: "#0184bc",
		chroma.LiteralStringRegex:  "#50a14f",
		chroma.LiteralNumber:       "#986801",
		chroma.Operator:            "#383a42",
		chroma.OperatorWord:        "#a626a4",
		chroma.Punctuation:         "#383a42",
		chroma.Comment:             "italic #a0a1a7",
		chroma.CommentPreproc:      "#4078f2",
		chroma.GenericDeleted:      "#e45649 bg:#f8d7da",
		chroma.GenericInserted:     "#50a14f bg:#d4edda",
		chroma.GenericHeading:      "bold #4078f2",
		chroma.GenericSubheading:   "bold #4078f2",
		chroma.GenericEmph:         "italic",
		chroma.GenericStrong:       "bold",
		chroma.Error:               "#e45649",
	}))
}
