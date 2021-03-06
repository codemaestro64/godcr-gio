package renderers

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/gomarkdown/markdown/ast"
	"github.com/planetdecred/godcr/ui/decredmaterial"
)

type labelFunc func(string) decredmaterial.Label

type Renderer struct {
	theme  *decredmaterial.Theme
	isList bool

	prefix         string
	padAccumulator []string
	leftPad        int
	links          map[string]*widget.Clickable
	stringBuilder  strings.Builder
	containers     []layout.Widget
	styleGroups    []map[string]string
	isHTML         bool

	table *tableRenderer
}

const (
	bulletUnicode = "\u2022"
	linkTag       = "[[link"
	linkSpacer    = "@@@@"
)

var (
	textBeforeCloseRegexp = regexp.MustCompile("(.*){/#}")
)

func newRenderer(theme *decredmaterial.Theme, isHTML bool) *Renderer {
	return &Renderer{
		theme:  theme,
		isHTML: isHTML,
	}
}

func (r *Renderer) pad() string {
	return strings.Repeat(" ", r.leftPad) + strings.Join(r.padAccumulator, "")
}

func (r *Renderer) addPad(pad string) {
	r.padAccumulator = append(r.padAccumulator, pad)
}

func (r *Renderer) popPad() {
	r.padAccumulator = r.padAccumulator[:len(r.padAccumulator)-1]
}

func (r *Renderer) RenderNode(w io.Writer, node ast.Node, entering bool) ast.WalkStatus {
	switch node := node.(type) {
	case *ast.Document:
		// Nothing to do
	case *ast.BlockQuote:

	case *ast.List:
		// extra new line at the end of a list *if* next is not a list
		if next := ast.GetNextNode(node); !entering && next != nil {
			_, parentIsListItem := node.GetParent().(*ast.ListItem)
			_, nextIsList := next.(*ast.List)
			if !nextIsList && !parentIsListItem {
				r.renderEmptyLine()
			}
		}
	case *ast.ListItem:
		r.renderList(node, entering)
	case *ast.Paragraph:
		if !entering {
			r.renderParagraph()
		}
	case *ast.Heading:
		if !entering {
			r.renderHeading(node.Level, true)
		}
	case *ast.Strong:
		r.renderStrong()
	case *ast.Link:
		if !entering {
			r.renderLink(node)
			return ast.SkipChildren
		}
	case *ast.Text:
		r.renderText(node)
	case *ast.Table:
		r.renderTable(entering)
	case *ast.TableCell:
		if !entering {
			r.renderTableCell(node)
		}
	case *ast.TableRow:
		r.renderTableRow(node, entering)
	}

	return ast.GoToNext
}

func (r *Renderer) renderStrong() {
	label := r.theme.Body1("")
	label.Font.Weight = text.Bold

	r.renderWords(label)
}

func (r *Renderer) renderParagraph() {
	r.renderWords(r.theme.Body1(""))
	// add dummy widget for new line
	r.renderEmptyLine()
}

func (r *Renderer) renderHeading(level int, block bool) {
	lblFunc := r.theme.H6

	switch level {
	case 1:
		lblFunc = r.theme.H4
	case 2:
		lblFunc = r.theme.H5
	case 3:
		lblFunc = r.theme.H6
	}

	r.renderWords(lblFunc(""))
	if block {
		// add dummy widget for new line
		r.renderEmptyLine()
	}
}

func (r *Renderer) renderLink(node *ast.Link) {
	dest := string(node.Destination)
	text := string(ast.GetFirstChild(node).AsLeaf().Literal)

	if r.links == nil {
		r.links = map[string]*widget.Clickable{}
	}

	if _, ok := r.links[dest]; !ok {
		r.links[dest] = new(widget.Clickable)
	}

	// fix a bug that causes the link to be written to the builder before this is called
	content := r.stringBuilder.String()
	r.stringBuilder.Reset()
	parts := strings.Split(content, " ")
	parts = parts[:len(parts)-1]
	for i := range parts {
		r.stringBuilder.WriteString(parts[i] + " ")
	}

	word := linkTag + linkSpacer + dest + linkSpacer + text
	r.stringBuilder.WriteString(word)
}

func (r *Renderer) renderText(node *ast.Text) {
	if string(node.Literal) == "\n" {
		return
	}

	content := string(node.Literal)
	if shouldCleanText(node) {
		content = removeLineBreak(content)
	}
	r.stringBuilder.WriteString(content)
}

func (r *Renderer) getNextChar(content string, currIndex int) byte {
	if currIndex+2 <= len(content) {
		return content[currIndex+1]
	}

	return 0
}

func (r *Renderer) renderWords(lbl decredmaterial.Label) {
	if r.isHTML {
		r.renderHTML(lbl)
		return
	}

	r.renderMarkdown(lbl)
}

func (r *Renderer) renderMarkdown(lbl decredmaterial.Label) {
	content := r.stringBuilder.String()
	r.stringBuilder.Reset()

	words := strings.Fields(content)
	words = append([]string{r.prefix}, words...)
	r.prefix = ""

	wdgt := func(gtx C) D {
		return decredmaterial.GridWrap{
			Axis:      layout.Horizontal,
			Alignment: layout.Start,
		}.Layout(gtx, len(words), func(gtx C, i int) D {
			if strings.HasPrefix(words[i], linkTag) {
				return r.getLinkWidget(gtx, words[i])
			}

			word := words[i] + " "
			if i == 0 {
				word = words[i]
			}
			lbl.Text = word
			return lbl.Layout(gtx)
		})
	}
	r.containers = append(r.containers, wdgt)
}

func (r *Renderer) getLabel(lbl decredmaterial.Label, text string) decredmaterial.Label {
	l := lbl
	l.Text = text
	l = r.styleLabel(l)
	return l
}

func (r *Renderer) renderHTML(lbl decredmaterial.Label) {
	content := r.stringBuilder.String()
	r.stringBuilder.Reset()

	var labels []decredmaterial.Label
	var inStyleBlock bool
	var isClosingStyle bool
	var isClosingBlock bool
	var currStyle string
	var currText string
	for i := range content {
		curr := content[i]

		if curr == openStyleTag[0] && r.getNextChar(content, i) == openStyleTag[1] {
			inStyleBlock = true
			labels = append(labels, r.getLabel(lbl, currText))
			currText = ""
		}

		if curr == halfCloseStyleTag[0] && r.getNextChar(content, i) == halfCloseStyleTag[1] {
			isClosingStyle = true
		}

		if curr == closeStyleTag[0] && r.getNextChar(content, i) == closeStyleTag[1] {
			isClosingBlock = true
		}

		if !inStyleBlock && !isClosingBlock {
			currStr := string(curr)
			currText += currStr

			if i+1 == len(content) || currStr == "" || currStr == " " {
				labels = append(labels, r.getLabel(lbl, currText))
				currText = ""
			}
		}

		if isClosingBlock && curr == closeStyleTag[3] {
			labels = append(labels, r.getLabel(lbl, currText))
			currText = ""
			r.removeLastStyleGroup()
			isClosingBlock = false

		}

		if inStyleBlock && !isClosingStyle {
			currStyle += string(curr)
		}

		if isClosingStyle && curr == halfCloseStyleTag[1] {
			isClosingStyle = false
			inStyleBlock = false
			r.addStyleGroup(currStyle)
			currStyle = ""
		}
	}

	wdgt := func(gtx C) D {
		return decredmaterial.GridWrap{
			Axis:      layout.Horizontal,
			Alignment: layout.Start,
		}.Layout(gtx, len(labels), func(gtx C, i int) D {
			return labels[i].Layout(gtx)
		})
	}
	r.containers = append(r.containers, wdgt)
}

func (r *Renderer) renderEmptyLine() {
	var padding = -5

	if r.isList {
		padding = -10
		r.isList = false
	}

	r.containers = append(r.containers, func(gtx C) D {
		dims := r.theme.Body2("").Layout(gtx)
		dims.Size.Y = dims.Size.Y + padding
		return dims
	})
}

func (r *Renderer) renderList(node *ast.ListItem, entering bool) {
	if entering {
		r.isList = true
		switch {
		// numbered list
		case node.ListFlags&ast.ListTypeOrdered != 0:
			itemNumber := 1
			siblings := node.GetParent().GetChildren()
			for _, sibling := range siblings {
				if sibling == node {
					break
				}
				itemNumber++
			}
			r.prefix += fmt.Sprintf("%d. ", itemNumber)

		// content of a definition
		case node.ListFlags&ast.ListTypeDefinition != 0:
			r.prefix += " "

		// no flags means it's the normal bullet point list
		default:
			r.prefix += " " + bulletUnicode + " "
		}
	}
}

func (r *Renderer) renderTable(entering bool) {
	if entering {
		r.table = newTableRenderer(r.theme)
	} else {
		r.containers = append(r.containers, r.table.Render())
		r.table = nil
	}
}

func (r *Renderer) renderTableCell(node *ast.TableCell) {
	content := r.stringBuilder.String()
	r.stringBuilder.Reset()

	align := CellAlignLeft
	switch node.Align {
	case ast.TableAlignmentRight:
		align = CellAlignRight
	case ast.TableAlignmentCenter:
		align = CellAlignCenter
	}

	if node.IsHeader {
		r.table.AddHeaderCell(content, align)
	} else {
		r.table.AddBodyCell(content, CellAlignCopyHeader)
	}
}

func (r *Renderer) renderTableRow(node *ast.TableRow, entering bool) {
	if _, ok := node.Parent.(*ast.TableBody); ok && entering {
		r.table.NextBodyRow()
	}
	if _, ok := node.Parent.(*ast.TableFooter); ok && entering {
		r.table.NextBodyRow()
	}
}

func (*Renderer) RenderHeader(w io.Writer, node ast.Node) {}

func (*Renderer) RenderFooter(w io.Writer, node ast.Node) {}

func (r *Renderer) Layout() ([]layout.Widget, map[string]*widget.Clickable) {
	return r.containers, r.links
}

func shouldCleanText(node ast.Node) bool {
	for node != nil {
		switch node.(type) {
		case *ast.BlockQuote:
			return false

		case *ast.Heading, *ast.Image, *ast.Link,
			*ast.TableCell, *ast.Document, *ast.ListItem:
			return true
		}
		node = node.GetParent()
	}

	return false
}

func removeLineBreak(text string) string {
	lines := strings.Split(text, "\n")

	if len(lines) <= 1 {
		return text
	}

	for i, l := range lines {
		switch i {
		case 0:
			lines[i] = strings.TrimRightFunc(l, unicode.IsSpace)
		case len(lines) - 1:
			lines[i] = strings.TrimLeftFunc(l, unicode.IsSpace)
		default:
			lines[i] = strings.TrimFunc(l, unicode.IsSpace)
		}
	}

	return strings.Join(lines, " ")
}

func (r *Renderer) getLinkWidget(gtx layout.Context, linkWord string) D {
	parts := strings.Split(linkWord, linkSpacer)

	gtx.Constraints.Max.X = gtx.Constraints.Max.X - 200
	return material.Clickable(gtx, r.links[parts[1]], func(gtx C) D {
		lbl := r.theme.Body2(parts[2] + " ")
		lbl.Color = r.theme.Color.Primary
		return lbl.Layout(gtx)
	})
}

type CellAlign int

const (
	CellAlignLeft CellAlign = iota
	CellAlignRight
	CellAlignCenter
	CellAlignCopyHeader
)

type tableCell struct {
	content       string
	alignment     CellAlign
	contentLength float64
}

type tableRenderer struct {
	header []tableCell
	body   [][]tableCell

	widths []float64
	theme  *decredmaterial.Theme
}

func newTableRenderer(theme *decredmaterial.Theme) *tableRenderer {
	return &tableRenderer{
		theme: theme,
	}
}

func (tr *tableRenderer) AddHeaderCell(content string, alignment CellAlign) {
	tr.header = append(tr.header, tableCell{
		content:       content,
		contentLength: float64(len(content)),
		alignment:     alignment,
	})
	tr.widths = append(tr.widths, 0)
}

func (tr *tableRenderer) NextBodyRow() {
	tr.body = append(tr.body, nil)
}

func (tr *tableRenderer) AddBodyCell(content string, alignement CellAlign) {
	row := tr.body[len(tr.body)-1]
	row = append(row, tableCell{
		content:       content,
		contentLength: float64(len(content)),
		alignment:     alignement,
	})
	tr.body[len(tr.body)-1] = row
}

// normalize ensure that the table has the same number of cells
// in each rows, header or not.
func (tr *tableRenderer) normalize() {
	width := len(tr.header)
	/**for _, row := range tr.body {
		//width = max(width, len(row))
	}**/

	// grow the header if needed
	for len(tr.header) < width {
		tr.header = append(tr.header, tableCell{})
	}

	// grow lines if needed
	for i := range tr.body {
		for len(tr.body[i]) < width {
			tr.body[i] = append(tr.body[i], tableCell{})
		}
	}
}

func (tr *tableRenderer) copyAlign() {
	for i, row := range tr.body {
		for j, cell := range row {
			if cell.alignment == CellAlignCopyHeader {
				tr.body[i][j].alignment = tr.header[j].alignment
			}
		}
	}
}

func (tr *tableRenderer) calculateLengths() {
	textLenghts := make([]float64, len(tr.header))

	for i := range tr.header {
		index := i
		textLenghts[index] = tr.header[index].contentLength
	}

	for i := range tr.body {
		index := i
		for k := range tr.body[index] {
			kIndex := k
			if textLenghts[kIndex] < tr.body[index][kIndex].contentLength {
				textLenghts[kIndex] = tr.body[index][kIndex].contentLength
			}
		}
	}

	total := float64(0)
	for i := range textLenghts {
		index := i
		total += textLenghts[index]
	}

	totalWidthRecouped := float64(0)
	cutWidths := []int{}
	for i := range textLenghts {
		index := i
		tr.widths[index] = (textLenghts[index] / total) * float64(100)
		if tr.widths[index] > 40 {
			totalWidthRecouped += tr.widths[index] - 40
			tr.widths[index] = 40
			cutWidths = append(cutWidths, index)
		}
	}

	averageWidthToAdd := totalWidthRecouped / float64(len(tr.widths)-len(cutWidths))
	for i := range tr.widths {
		index := i
		for k := range cutWidths {
			kIndex := k
			if index == kIndex {
				continue
			}
			tr.widths[index] += averageWidthToAdd
		}
	}
}

func (tr *tableRenderer) Render() layout.Widget {
	var tableChildren []layout.FlexChild
	tr.normalize()
	tr.copyAlign()

	tr.calculateLengths()

	if tr.header != nil {
		header := tr.getTableRow(tr.header)
		tableChildren = append(tableChildren, layout.Rigid(header))
	}

	for i := range tr.body {
		index := i
		row := tr.getTableRow(tr.body[index])
		tableChildren = append(tableChildren, layout.Rigid(row))
	}

	return func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, tableChildren...)
	}
}

func (tr *tableRenderer) getTableRow(row []tableCell) layout.Widget {
	children := make([]layout.FlexChild, len(row))
	for i := range row {
		index := i
		children[index] = layout.Rigid(func(gtx C) D {
			gtx.Constraints.Max.X = int((tr.widths[index] / 100) * float64(gtx.Constraints.Max.X))
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return tr.theme.Body2(row[index].content).Layout(gtx)
		})
	}

	return func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		dims := layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx, children...)
		dims.Size.Y += 5
		return dims
	}
}
