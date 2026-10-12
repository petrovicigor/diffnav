package diffviewer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/log/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	tint "github.com/lrstanley/bubbletint/v2"

	"github.com/dlvhdr/diffnav/pkg/filenode"
	"github.com/dlvhdr/diffnav/pkg/icons"
	"github.com/dlvhdr/diffnav/pkg/ui/common"
	"github.com/dlvhdr/diffnav/pkg/utils"
	"github.com/robinovitch61/viewport/filterableviewport"
	"github.com/robinovitch61/viewport/viewport"
	"github.com/robinovitch61/viewport/viewport/item"
)

const dirHeaderHeight = 3

type cachedNode struct {
	path      string
	files     []*gitdiff.File
	additions int64
	deletions int64
	diff      []diffLine
	// A finished render (the nodes in the cache and on screen): key is its
	// renderKey, isDir whether it is a directory view, and fileStarts[i]
	// the line where its i-th file's render starts in diff (filePaths[i]
	// is that file's name; directory views only).
	key        string
	isDir      bool
	fileStarts []int
	filePaths  []string
}

type nodeCache map[string]*cachedNode

// renderKey names a render of node: the view kind, its path, and all that
// shapes delta's output (width, theme, layout). A render is shown or reused
// only under the key it was asked for.
func (m *Model) renderKey(node *cachedNode, dir bool) string {
	kind, sbs := "file", m.sideBySide
	if dir {
		kind = "dir"
	} else if f := node.files[0]; f.IsNew || f.IsDelete {
		sbs = false
	}
	return fmt.Sprintf("%s:%s:%d:%s:%t", kind, node.path, m.contentWidth(), common.Themes.Current().ID, sbs)
}

type diffLine struct {
	item item.Item
}

func (o diffLine) GetItem() item.Item {
	return o.item
}

var ViewportKeyMap = viewport.KeyMap{
	HalfPageDown: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "scroll half page down"),
	),
	HalfPageUp: key.NewBinding(
		key.WithKeys("ctrl+u"),
		key.WithHelp("ctrl+u", "scroll half page up"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "prev file"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "next file"),
	),
	Bottom: key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("G", "bottom"),
	),
	Top: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "top"),
	),
	Left: key.NewBinding(
		key.WithKeys("left"),
		key.WithHelp("←", "scroll left"),
	),
	Right: key.NewBinding(
		key.WithKeys("right"),
		key.WithHelp("→", "scroll right"),
	),
}

type Model struct {
	common.Common
	fvp        *filterableviewport.Model[diffLine]
	file       *cachedNode
	dir        *cachedNode
	cache      nodeCache
	sideBySide bool
	preamble   string
	sb         common.Scrollbar
	// jump remembers the last file jump (JumpFile), see currentFileIdx.
	jump fileJump
	// shown is the render on screen. file / dir are what was asked for
	// (header and stats); until its render lands, shown is the previous one.
	shown *cachedNode
	// gen numbers render requests: a render lands only if no newer request
	// (another node, width, theme or layout) was made since.
	gen uint64
	// cancel stops the render running (nil when none).
	cancel context.CancelFunc
	// rootAdded / rootDeleted are the stats of the whole diff ("/").
	rootAdded, rootDeleted int64
}

// SetPreamble stores the preamble text (e.g. commit metadata from git show).
func (m *Model) SetPreamble(preamble string) {
	m.preamble = preamble
}

func viewportStyles(styles *common.Styles) viewport.Styles {
	return viewport.Styles{
		SelectionPrefix: lipgloss.NewStyle().Foreground(styles.Tint.Blue).Render("▐"),
		SelectedItemStyle: lipgloss.NewStyle().
			Background(styles.Colors.SelectionBg).
			Foreground(styles.Tint.BrightWhite),
		FooterStyle: lipgloss.NewStyle().
			BorderForeground(styles.Tint.BrightBlack).
			Background(styles.Tint.BrightBlack).
			Border(lipgloss.Border{Left: "", Right: ""}, false, true, false, true).
			Foreground(styles.Colors.PillFg).
			Italic(true),
	}
}

func scrollbarStyles(styles *common.Styles) common.ScrollbarStyles {
	return common.ScrollbarStyles{
		Thumb: lipgloss.NewStyle().Foreground(styles.Tint.Blue),
		Track: lipgloss.NewStyle().Foreground(styles.Tint.Black),
	}
}

func filterableStyles(styles *common.Styles) filterableviewport.Styles {
	s := filterableviewport.DefaultStyles()
	s.Filter.Focused.TextInput.Text = lipgloss.NewStyle().
		Foreground(styles.Tint.BrightWhite)
	s.Filter.Unfocused.TextInput.Text = lipgloss.NewStyle().
		Foreground(styles.Tint.White)
	s.Filter.Empty = lipgloss.NewStyle().Foreground(styles.Tint.White)
	s.MatchesCount.Matches = lipgloss.NewStyle().
		Foreground(styles.Tint.BrightYellow)
	s.Match.Focused = lipgloss.NewStyle().
		Background(styles.Colors.MatchFocusedBg).
		Foreground(styles.Colors.MatchFocusedFg)
	s.Match.Unfocused = lipgloss.NewStyle().
		Background(styles.Colors.MatchBg).
		Foreground(styles.Colors.MatchFg)
	return s
}

func New(sideBySide bool, styles *common.Styles) Model {
	vp := viewport.New(
		0,
		0,
		viewport.WithKeyMap[diffLine](ViewportKeyMap),
		viewport.WithStyles[diffLine](viewportStyles(styles)),
	)

	sb := common.Scrollbar{Styles: scrollbarStyles(styles)}

	filterableViewportKeyMap := filterableviewport.DefaultKeyMap()
	filterableViewportKeyMap.CancelFilterKey = key.NewBinding(
		key.WithKeys("esc", "ctrl+c"),
		key.WithHelp("esc/ctrl+c", "cancel filter"),
	)

	filterableViewportStyles := filterableStyles(styles)

	return Model{
		Common: common.Common{
			Styles: styles,
		},
		sb: sb,
		fvp: filterableviewport.New(
			vp,
			filterableviewport.WithKeyMap[diffLine](filterableViewportKeyMap),
			filterableviewport.WithStyles[diffLine](filterableViewportStyles),
			filterableviewport.WithPrefixText[diffLine](
				lipgloss.NewStyle().Bold(true).Render("Filter:"),
			),
			filterableviewport.WithFilterModes[diffLine](
				[]filterableviewport.FilterMode{
					filterableviewport.CaseInsensitiveFilterMode(
						key.NewBinding(
							key.WithKeys("/"),
							key.WithHelp("/", "case insensitive"),
						),
					), filterableviewport.RegexFilterMode(
						key.NewBinding(
							key.WithKeys("ctrl+r"),
							key.WithHelp("ctrl+r", "regex fitler mode"),
						),
					),
					filterableviewport.FuzzyFilterMode(
						key.NewBinding(
							key.WithKeys("ctrl+f"),
							key.WithHelp("ctrl+f", "fuzzy fitler mode"),
						),
					),
					filterableviewport.ExactFilterMode(key.NewBinding(
						key.WithKeys("ctrl+s"),
						key.WithHelp("ctrl+s", "exact"),
					)),
				},
			),
			filterableviewport.WithPlaceholderText[diffLine]("type to search…"),
			filterableviewport.WithItemDescriptor[diffLine]("lines"),
			// No idle hint: the row stays (the filter line is always
			// reserved) but blank until / opens a search; ? lists the modes.
			filterableviewport.WithEmptyText[diffLine](""),
			filterableviewport.WithFilterLinePosition[diffLine](filterableviewport.FilterLineTop),
			filterableviewport.WithMatchingItemsOnly[diffLine](false),
			filterableviewport.WithCanToggleMatchingItemsOnly[diffLine](true),
			filterableviewport.WithVerticalPad[diffLine](8),
			filterableviewport.WithHorizontalPad[diffLine](8),
		),
		sideBySide: sideBySide,
		cache:      map[string]*cachedNode{},
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	log.Debug("diffviewer", "msg", fmt.Sprintf("%T", msg))
	cmds := make([]tea.Cmd, 0)
	switch msg := msg.(type) {
	case diffContentMsg:
		// A late render (for a node, width, theme or layout no longer
		// asked for) is dropped: it would replace what is on screen.
		if msg.gen == m.gen {
			node := &cachedNode{
				path:       msg.path,
				diff:       msg.lines,
				key:        msg.key,
				isDir:      msg.isDir,
				fileStarts: msg.fileStarts,
				filePaths:  msg.filePaths,
			}
			m.cache[msg.key] = node
			m.show(node)
		}
	case renderDueMsg:
		if msg.gen == m.gen {
			cmds = append(cmds, m.diff())
		}
	}

	fvp, fvpCmd := m.fvp.Update(msg)
	cmds = append(cmds, fvpCmd)
	m.fvp = fvp

	return m, tea.Batch(cmds...)
}

// scrollbarWidth is what the viewer's width holds besides the diff: a gap
// and the scrollbar (sideColumn), plus one column the layout spends on the
// tree's separator without taking it off the width it hands the viewer.
const scrollbarWidth = 3

// View is the viewport with a gap and a column beside it: the scrollbar
// next to the diff lines only, the header's rule carried through to the
// edge, and blanks beside the header text, the search line and the footer.
// (The scrollbar used to run the full height, so the header rule ran into
// its track and its thumb sat beside the file name; and with no gap, every
// full-width delta rule butted into the track.)
func (m Model) View() string {
	lines := strings.Split(m.fvp.View(), "\n")
	top, bottom := m.headerRows()+1, 1 // header, search line | footer
	track := max(len(lines)-top-bottom, 0)
	itemMetrics := m.fvp.GetItemMetrics()
	scrollbar := m.sb.View(
		track,
		itemMetrics.TotalItems,
		itemMetrics.FirstVisibleItemIdx,
		itemMetrics.LastVisibleItemIdx,
	)
	rule := lipgloss.NewStyle().Foreground(m.Styles.Tint.Black).Render("──")
	return strings.Join(
		sideColumn(lines, m.contentWidth(), top, scrollbar, m.RuleRow(), rule),
		"\n",
	)
}

// sideColumn pads every line to width and appends two cells: a gap and the
// scrollbar's row from row top on (none when it is ""); rule (two cells) on
// ruleRow; blanks elsewhere.
func sideColumn(lines []string, width, top int, scrollbar string, ruleRow int, rule string) []string {
	var bar []string
	if scrollbar != "" {
		bar = strings.Split(scrollbar, "\n")
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		cell := "  "
		switch {
		case i == ruleRow:
			cell = rule
		case i >= top && i-top < len(bar):
			cell = " " + bar[i-top]
		}
		if pad := width - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		out[i] = line + cell
	}
	return out
}

// headerRows is how many lines the header takes in the viewport (an empty
// header is still one blank line there).
func (m Model) headerRows() int {
	return len(strings.Split(m.headerView(), "\n"))
}

// RuleRow is the row of the header's bottom rule in View, or -1 with no
// header. The layout joins the tree's separator to it there.
func (m Model) RuleRow() int {
	if m.headerView() == "" {
		return -1
	}
	return m.headerRows() - 1
}

func (m *Model) SetSize(width, height int) tea.Cmd {
	first, resized := m.Width == 0, width != m.Width
	m.Width = width
	m.Height = height
	m.fvp.SetWidth(m.contentWidth())
	m.fvp.SetHeight(m.Height)
	// The header is pre-rendered at a width: without this its rule kept the
	// old width when the tree was hidden or shown.
	m.updateHeader()
	switch {
	case !resized:
		return nil // the height does not change a render
	case first:
		return m.diff()
	}
	return m.diffSettled()
}

// resizeDebounce is how long a new width must hold before it is rendered.
var resizeDebounce = 80 * time.Millisecond

// renderDueMsg asks for the render of a width that has held (diffSettled).
type renderDueMsg struct{ gen uint64 }

// diffSettled is diff for a width that may still be changing (a window
// edge being dragged): a cached render shows at once; anything else renders
// once no other request came for resizeDebounce, so a resize storm runs
// delta once instead of once per step. The render on screen stays meanwhile.
func (m *Model) diffSettled() tea.Cmd {
	node, dir := m.asked()
	if node == nil {
		return nil
	}
	if _, ok := m.cache[m.renderKey(node, dir)]; ok {
		return m.diff()
	}
	m.stopRender()
	gen := m.gen
	return tea.Tick(resizeDebounce, func(time.Time) tea.Msg { return renderDueMsg{gen: gen} })
}

// UpdateTheme re-styles the viewer with styles (the theme's new Styles) and
// re-renders the diff.
func (m *Model) UpdateTheme(styles *common.Styles) tea.Cmd {
	if styles != nil {
		m.Styles = styles
	}
	m.fvp.SetViewportStyles(viewportStyles(m.Styles))
	m.fvp.SetFilterableViewportStyles(filterableStyles(m.Styles))
	m.sb.Styles = scrollbarStyles(m.Styles)
	m.updateHeader()
	return m.diff()
}

func (m Model) contentWidth() int {
	return m.Width - scrollbarWidth
}

// asked is what was asked for: the file, else the directory (dir true).
func (m *Model) asked() (node *cachedNode, dir bool) {
	if m.file != nil {
		return m.file, false
	}
	return m.dir, m.dir != nil
}

// diff shows the render of what was asked for (file or dir) at the current
// width, theme and layout: from the cache when it was rendered before, or
// by starting its render. Either way any render still running is cancelled.
func (m *Model) diff() tea.Cmd {
	node, dir := m.asked()
	if node == nil || m.contentWidth() <= 0 || (!dir && len(node.files) != 1) {
		return nil
	}
	key := m.renderKey(node, dir)
	m.stopRender()
	if cached, ok := m.cache[key]; ok {
		m.show(cached)
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	return m.newRender(node, dir, key).cmd(ctx)
}

// stopRender cancels the render running (its delta processes are killed)
// and makes any render already on its way stale.
func (m *Model) stopRender() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.gen++
}

// show puts a finished render on screen.
func (m *Model) show(node *cachedNode) {
	m.shown = node
	m.jump = fileJump{} // its file offsets belong to the old render
	m.fvp.SetObjects(node.diff)
}

func (m Model) headerView() string {
	if m.dir != nil {
		return m.dirHeaderView()
	}

	if m.file == nil || len(m.file.files) != 1 {
		return ""
	}
	name := m.file.path
	base := lipgloss.NewStyle()

	fileIcon := icons.GetIcon(name, false)
	prefix := base.Render(fileIcon) + base.Render(" ")
	name = utils.TruncateString(name, m.contentWidth()-lipgloss.Width(prefix))
	top := prefix + base.Bold(true).Render(name)

	bottom := filenode.ViewFileDiffStats(m.file.files[0], base)

	return base.
		Width(m.contentWidth()).
		Height(dirHeaderHeight - 1).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(m.Styles.Tint.Black).
		Render(lipgloss.JoinVertical(lipgloss.Left, top, bottom))
}

func (m Model) dirHeaderView() string {
	base := lipgloss.NewStyle().Foreground(m.Styles.Tint.Blue)
	prefix := base.Render(" ")
	name := utils.TruncateString(m.dir.path, m.contentWidth()-lipgloss.Width(prefix))

	top := prefix + base.Bold(true).Render(name)
	bottom := filenode.ViewDiffStats(m.dir.additions, m.dir.deletions, base)
	return base.
		Width(m.contentWidth()).
		Height(dirHeaderHeight - 1).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(m.Styles.Tint.Black).
		Render(lipgloss.JoinVertical(lipgloss.Left, top, bottom))
}

func (m Model) SetFilePatch(file *gitdiff.File) (Model, tea.Cmd) {
	m.dir = nil
	additions, deletions := filenode.DiffStats(file)
	m.file = &cachedNode{
		path:      filenode.GetFileName(file),
		files:     []*gitdiff.File{file},
		additions: additions,
		deletions: deletions,
	}
	m.updateHeader()
	return m, m.diff()
}

func (m Model) SetDirPatch(dirPath string, files []*gitdiff.File) (Model, tea.Cmd) {
	m.file = nil
	var added, deleted int64
	for _, file := range files {
		na, nd := filenode.DiffStats(file)
		added += na
		deleted += nd
	}
	m.dir = &cachedNode{
		path:      dirPath,
		files:     files,
		additions: added,
		deletions: deleted,
	}
	if dirPath == "/" {
		m.rootAdded, m.rootDeleted = added, deleted
	}
	m.updateHeader()
	return m, m.diff()
}

func (m *Model) updateHeader() {
	m.fvp.SetHeader(strings.Split(m.headerView(), "\n"))
}

// SetSideBySide updates the diff view mode and re-renders.
func (m *Model) SetSideBySide(sideBySide bool) tea.Cmd {
	m.sideBySide = sideBySide
	return m.diff()
}

// ScrollUp scrolls the viewport up by the given number of lines.
func (m *Model) ScrollUp(lines int) {
	m.fvp.ScrollUp(lines)
}

// ScrollDown scrolls the viewport down by the given number of lines.
func (m *Model) ScrollDown(lines int) {
	m.fvp.ScrollDown(lines)
}

// ScrollBottom scrolls the viewport to the bottom.
func (m *Model) ScrollBottom() {
	m.fvp.GoToBottom()
}

// ScrollTop scrolls the viewport to its top.
func (m *Model) ScrollTop() {
	m.fvp.GoToTop()
}

// ScrollLeft moves the viewport to the left by the given number of columns.
func (m *Model) ScrollLeft(cols int) {
	m.fvp.ScrollLeft(cols)
}

// ScrollRight scrolls the viewport one column away from column 0.
func (m *Model) ScrollRight(cols int) {
	m.fvp.ScrollRight(cols)
}
func (m Model) renderPreamble(preamble string) string {
	preamble = strings.TrimSpace(preamble)
	if preamble == "" {
		return ""
	}

	dim := lipgloss.NewStyle().Foreground(m.Styles.Tint.Black)
	yellow := lipgloss.NewStyle().Foreground(m.Styles.Tint.Yellow)

	var out []string
	for line := range strings.SplitSeq(preamble, "\n") {
		switch {
		case strings.HasPrefix(line, "commit "):
			out = append(
				out,
				dim.Render("commit ")+yellow.Render(strings.TrimPrefix(line, "commit ")),
			)
		case strings.HasPrefix(line, "Author:"),
			strings.HasPrefix(line, "AuthorDate:"),
			strings.HasPrefix(line, "Date:"),
			strings.HasPrefix(line, "Commit:"),
			strings.HasPrefix(line, "CommitDate:"),
			strings.HasPrefix(line, "Merge:"):
			out = append(out, dim.Render(line))
		default:
			out = append(out, line)
		}
	}

	return strings.Join(out, "\n")
}

// ClearCache forgets every render (the diff changed): nothing rendered
// before, or still rendering, is shown again.
func (m *Model) ClearCache() {
	m.cache = make(nodeCache)
	m.stopRender()
}

func (m *Model) RootDiffStats() (int64, int64) {
	return m.rootAdded, m.rootDeleted
}

func (m *Model) Searching() bool {
	return m.fvp.FilterFocused()
}

func (m *Model) SetFiltering() {
	var mode filterableviewport.FilterModeName
	if curr := m.fvp.GetActiveFilterMode(); curr != nil {
		mode = curr.Name
	} else {
		mode = filterableviewport.FilterExact
	}
	m.fvp.SetFilter(m.fvp.GetFilterText(), mode)
}

func (m *Model) Filtering() bool {
	return m.fvp.GetActiveFilterMode() != nil
}

func (m *Model) SelectionEnabled() bool {
	return m.fvp.GetSelectionEnabled()
}

func (m *Model) SetSelectionEnabled(val bool) {
	m.fvp.SetSelectionEnabled(val)
}

func stringToDiffLines(val string) []diffLine {
	lines := strings.Split(val, "\n")
	objects := make([]diffLine, len(lines))
	for i, line := range lines {
		objects[i] = diffLine{item: item.NewItem(line)}
	}
	return objects
}

type deltaOpts struct {
	IsNew    bool
	IsDelete bool
	// OmitFileHeader drops delta's file header: the single-file view already
	// names the file in its own header.
	OmitFileHeader bool
}

// dirFileKind is how a file in a directory view is laid out by delta.
type dirFileKind int

const (
	dirFileChanged dirFileKind = iota
	dirFileNew
	dirFileDeleted
)

func dirFileKindOf(file *gitdiff.File) dirFileKind {
	switch {
	case file.IsNew:
		return dirFileNew
	case file.IsDelete:
		return dirFileDeleted
	}
	return dirFileChanged
}

// deltaLayout is the side-by-side switch and options for a file of kind in
// a directory view: the same as its single-file view, so a new or deleted
// file is one column (not a side-by-side with an empty half), but keeping
// its file header.
func (k dirFileKind) deltaLayout(sideBySide bool) (bool, deltaOpts) {
	switch k {
	case dirFileNew:
		return false, deltaOpts{IsNew: true}
	case dirFileDeleted:
		return false, deltaOpts{IsDelete: true}
	}
	return sideBySide, deltaOpts{}
}

// singleFileOpts are the delta options for file shown on its own. A rename or
// copy keeps delta's file header, the only place "old ⟶ new" is shown.
func singleFileOpts(file *gitdiff.File) deltaOpts {
	return deltaOpts{
		IsNew:          file.IsNew,
		IsDelete:       file.IsDelete,
		OmitFileHeader: !file.IsRename && !file.IsCopy,
	}
}

// deltaFeature is the gitconfig delta feature that styles the diff for
// theme ("" = diffnav styles delta itself). The Miasma tints hand all
// colours to the user's own miasma-dark / miasma-light delta features, when
// the gitconfig has them (hasDeltaFeature).
func deltaFeature(theme *tint.Tint) string {
	if !common.IsMiasma(theme) {
		return ""
	}
	name := "miasma-light"
	if theme.Dark {
		name = "miasma-dark"
	}
	if !hasDeltaFeature(name) {
		return ""
	}
	return name
}

// featureHeaderArgs shape delta's file and hunk headers for a pane that is
// not a pager. Text only, no colours: the feature still styles both.
//
//   - Empty file labels: navigate=true (gitconfig) prefixes every file
//     header with "Δ" / "added:" / "removed:" / "renamed:", markers for
//     less's n/N that mean nothing here (the tree shows the file's status).
//     A rename still reads "old ⟶ new".
//   - Empty hunk label: navigate's "•" before each hunk header, same reason.
//   - Hunk headers without the file: the file header right above already
//     names it, so "line-number syntax" (the feature's "file line-number
//     syntax" minus the path) keeps only the line and the code context.
var featureHeaderArgs = []string{
	"--file-modified-label=",
	"--file-added-label=",
	"--file-removed-label=",
	"--file-renamed-label=",
	"--file-copied-label=",
	"--hunk-label=",
	"--hunk-header-style=line-number syntax",
}

// hunkRuleColor is the faint rule drawn above each hunk header. The feature's
// own "box ul" draws a box with no left side plus an underline across the
// pane, which reads as broken chrome inside diffnav's frame; one full-width
// rule in the separators' faint colour reads as a section break instead.
func hunkRuleColor(feature string) string {
	if feature == "miasma-light" {
		return "#c8bd9f"
	}
	return "#3a352d"
}

// featureDeltaArgs runs delta with feature for every colour and style,
// passing only layout: no pager, line numbers (one column for new / deleted
// files), tab width, no wrap symbols, the pane's width, no wrapping limit,
// side-by-side when asked, and the header shape (featureHeaderArgs; no file
// header at all with OmitFileHeader).
func featureDeltaArgs(feature string, sideBySide bool, width int, opts deltaOpts) []string {
	args := []string{
		"--features=" + feature,
		"--paging=never",
		"--line-numbers",
		"--tabs=2",
		"--wrap-left-symbol= ",
		"--wrap-right-symbol= ",
		"--wrap-right-prefix-symbol= ",
		fmt.Sprintf("-w=%d", width),
		"--max-line-length=0",
		"--wrap-max-lines=unlimited",
	}
	args = append(args, featureHeaderArgs...)
	args = append(args, "--hunk-header-decoration-style="+hunkRuleColor(feature)+" ol")
	if opts.OmitFileHeader {
		args = append(args, "--file-style=omit")
	}
	switch {
	case opts.IsNew:
		args = append(args, "--line-numbers-left-format=", "--line-numbers-right-format={np:>4} ")
	case opts.IsDelete:
		args = append(args, "--line-numbers-left-format={nm:>4} ", "--line-numbers-right-format=")
	default:
		args = append(args, "--line-numbers-left-format={nm:>4} ", "--line-numbers-right-format={np:>4} ")
	}
	if sideBySide {
		args = append(args, "--side-by-side")
	}
	return args
}

// makeDeltaArgs are the args delta renders with: the theme's styling and
// layout. A unified render also needs deltaEnv(…, false), which keeps a
// gitconfig's side-by-side = true from overriding the layout asked for.
func (m *Model) makeDeltaArgs(sideBySide bool, width int, opts deltaOpts) []string {
	theme := common.Themes.Current()
	if feature := deltaFeature(theme); feature != "" {
		return featureDeltaArgs(feature, sideBySide, width, opts)
	}
	sSelection := lipgloss.NewStyle().Background(m.Styles.Colors.SelectionBg)
	selectionColor := common.LipglossColorToHex(m.Styles.Colors.SelectionBg)
	white := common.LipglossColorToHex(theme.White)
	green := common.LipglossColorToHex(lipgloss.Darken(theme.Green, 0.8))
	brightGreen := common.LipglossColorToHex(
		lipgloss.Darken(theme.BrightGreen, 0.5),
	)
	plusLine := common.LipglossColorToHex(theme.BrightGreen)
	minusLine := common.LipglossColorToHex(theme.BrightRed)
	red := common.LipglossColorToHex(lipgloss.Darken(theme.Red, 0.8))
	brightRed := common.LipglossColorToHex(lipgloss.Darken(theme.BrightRed, 0.5))
	args := []string{
		"--paging=never",
		"--line-numbers",
		"--tabs=2",
		"--wrap-left-symbol= ",
		"--wrap-right-symbol= ",
		"--wrap-right-prefix-symbol= ",
		"--hunk-label=  󰡏 ",
		fmt.Sprintf(
			"--syntax-theme=%s",
			common.SupportedThemeToDeltaSyntax[theme.ID],
		),
		fmt.Sprintf("--file-modified-label=%s",
			utils.RemoveReset(sSelection.Foreground(m.Styles.Tint.BrightYellow).Render(" "))),
		fmt.Sprintf("--file-removed-label=%s",
			utils.RemoveReset(sSelection.Foreground(m.Styles.Tint.BrightRed).Render(" "))),
		fmt.Sprintf("--file-added-label=%s",
			utils.RemoveReset(sSelection.Foreground(m.Styles.Tint.BrightGreen).Render(" "))),
		fmt.Sprintf("--file-renamed-label=%s",
			utils.RemoveReset(sSelection.Foreground(m.Styles.Tint.Blue).Render("renamed:"))),
		fmt.Sprintf("--file-style='\"%s\" bold \"%s\"'", selectionColor, selectionColor),
		fmt.Sprintf("--file-decoration-style='\"%s\" box %s'", selectionColor, selectionColor),
		fmt.Sprintf("-w=%d", width),
		"--max-line-length=0",
		"--wrap-max-lines=unlimited",
		fmt.Sprintf(
			"--line-numbers-right-style='\"%s\" dim'",
			common.LipglossColorToHex(m.Styles.Colors.FaintBlue()),
		),
		fmt.Sprintf("--line-numbers-zero-style='\"%s\" dim'", white),
		fmt.Sprintf("--plus-style='syntax \"%s\"'", green),
		fmt.Sprintf("--plus-emph-style='syntax \"%s\"'", brightGreen),
		fmt.Sprintf("--minus-style='syntax \"%s\"'", red),
		fmt.Sprintf("--minus-emph-style='syntax \"%s\"'", brightRed),
		fmt.Sprintf(
			"--hunk-header-line-number-style='%s'",
			common.LipglossColorToHex(m.Styles.Colors.FaintBlue()),
		),
		fmt.Sprintf(
			"--hunk-header-style='%s'",
			white,
		),
		fmt.Sprintf("--hunk-header-file-style='\"%s\" dim'", white),
		fmt.Sprintf(
			"--hunk-header-decoration-style='\"%s\" ol ul'",
			common.LipglossColorToHex(m.Styles.Colors.FaintBlue()),
		),
	}

	if opts.IsNew {
		args = append(
			args,
			fmt.Sprintf("--line-numbers-plus-style='\"%s\" \"%s\"'", plusLine, green),
			"--line-numbers-minus-style=",
			"--line-numbers-left-format=",
			"--line-numbers-right-format={np:>4} ",
		)
	} else if opts.IsDelete {
		args = append(
			args,
			"--line-numbers-plus-style=",
			fmt.Sprintf("--line-numbers-minus-style=\"%s\" \"%s\"", minusLine, red),
			"--line-numbers-left-format={nm:>4} ",
			"--line-numbers-right-format=",
		)
	} else {
		args = append(
			args,
			fmt.Sprintf("--line-numbers-plus-style='\"%s\" \"%s\"'", plusLine, green),
			fmt.Sprintf("--line-numbers-minus-style='\"%s\" \"%s\"", minusLine, red),
			"--line-numbers-left-format={nm:>4} ",
			"--line-numbers-right-format={np:>4} ",
		)
	}

	if sideBySide {
		args = append(args, "--side-by-side")
	}
	if theme.Dark {
		args = append(args, "--dark")
	} else {
		args = append(args, "--light")
	}

	return args
}
