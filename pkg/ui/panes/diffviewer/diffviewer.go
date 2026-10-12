package diffviewer

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

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
	// fileStarts[i] is the line where files[i]'s render starts in diff
	// (directory views only; filePaths[i] is its name).
	fileStarts []int
	filePaths  []string
}

type nodeCache map[string]*cachedNode

func cacheKey(path string, sideBySide bool) string {
	key := path + ":" + common.Themes.Current().ID
	if sideBySide {
		return key + ":sbs"
	}
	return key
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
			Foreground(styles.Tint.White).
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
			filterableviewport.WithEmptyText[diffLine](
				"Search… "+lipgloss.NewStyle().
					Faint(true).
					Render("(/ insensitive ⋅ ⌃+s exact ⋅ ⌃+r regex ⋅ ⌃+f fuzzy)"),
			),
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
		if node, ok := m.cache[msg.cacheKey]; ok {
			node.diff = msg.lines
			node.fileStarts = msg.fileStarts
			node.filePaths = msg.filePaths
		}
		m.fvp.SetObjects(msg.lines)
	}

	fvp, fvpCmd := m.fvp.Update(msg)
	cmds = append(cmds, fvpCmd)
	m.fvp = fvp

	return m, tea.Batch(cmds...)
}

const scrollbarWidth = 2 // 1 scrollbar character + 1 padding

func (m Model) View() string {
	vpView := m.fvp.View()
	itemMetrics := m.fvp.GetItemMetrics()
	scrollbar := m.sb.View(
		m.fvp.GetHeight(),
		itemMetrics.TotalItems,
		itemMetrics.FirstVisibleItemIdx,
		itemMetrics.LastVisibleItemIdx,
	)
	if scrollbar != "" {
		vpView = lipgloss.JoinHorizontal(lipgloss.Top, vpView, scrollbar)
	}
	return vpView
}

func (m *Model) SetSize(width, height int) tea.Cmd {
	m.Width = width
	m.Height = height
	m.fvp.SetWidth(m.contentWidth())
	m.fvp.SetHeight(m.Height)
	m.ClearCache()
	return m.diff()
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
	m.ClearCache()
	m.updateHeader()
	return m.diff()
}

func (m Model) contentWidth() int {
	return m.Width - scrollbarWidth
}

func (m *Model) diff() tea.Cmd {
	if m.file != nil {
		file := m.file.files[0]
		sideBySide := m.sideBySide && !file.IsNew && !file.IsDelete
		key := cacheKey(m.file.path, sideBySide)
		if cached, ok := m.cache[key]; ok && len(cached.diff) != 0 {
			m.file = cached
			m.fvp.SetObjects(cached.diff)
			return nil
		}
		node := &cachedNode{
			path:      m.file.path,
			files:     m.file.files,
			additions: m.file.additions,
			deletions: m.file.deletions,
		}
		m.file = node
		m.cache[key] = node
		return m.diffFile(node, m.contentWidth())
	} else if m.dir != nil {
		key := cacheKey(m.dir.path, m.sideBySide)
		if cached, ok := m.cache[key]; ok && len(cached.diff) != 0 {
			m.dir = cached
			m.fvp.SetObjects(cached.diff)
			return nil
		}
		node := &cachedNode{
			path:      m.dir.path,
			files:     m.dir.files,
			additions: m.dir.additions,
			deletions: m.dir.deletions,
		}
		m.dir = node
		m.cache[key] = node
		preamble := ""
		if m.dir.path == "/" {
			preamble = m.preamble
		}
		return m.diffDir(node, m.contentWidth(), m.sideBySide, preamble)
	}

	return nil
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

	fname := filenode.GetFileName(file)
	sideBySide := m.sideBySide && !file.IsNew && !file.IsDelete
	key := cacheKey(fname, sideBySide)
	if cached, ok := m.cache[key]; ok {
		m.file = cached
		m.updateHeader()
		m.fvp.SetObjects(cached.diff)
		return m, nil
	}

	files := make([]*gitdiff.File, 1)
	files[0] = file
	additions, deletions := filenode.DiffStats(file)
	m.file = &cachedNode{
		path:      fname,
		files:     files,
		additions: additions,
		deletions: deletions,
	}
	m.cache[key] = m.file

	m.updateHeader()
	return m, m.diffFile(m.file, m.contentWidth())
}

func (m Model) SetDirPatch(dirPath string, files []*gitdiff.File) (Model, tea.Cmd) {
	m.file = nil

	key := cacheKey(dirPath, m.sideBySide)
	if cached, ok := m.cache[key]; ok {
		m.dir = cached
		m.updateHeader()
		m.fvp.SetObjects(cached.diff)
		return m, nil
	}

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
	m.cache[key] = m.dir
	preamble := ""
	if dirPath == "/" {
		preamble = m.preamble
	}
	m.updateHeader()
	return m, m.diffDir(m.dir, m.contentWidth(), m.sideBySide, preamble)
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

func (m *Model) diffFile(node *cachedNode, width int) tea.Cmd {
	if width == 0 || node == nil || len(node.files) != 1 {
		return nil
	}

	file := node.files[0]
	sideBySide := m.sideBySide && !file.IsNew && !file.IsDelete
	key := cacheKey(node.path, sideBySide)
	return func() tea.Msg {
		args := m.makeDeltaArgs(sideBySide, width, singleFileOpts(file))
		log.Info("executing delta", "cmd", fmt.Sprintf(`delta %s`, strings.Join(args, " ")))
		deltac := exec.Command("delta", args...)
		deltac.Env = os.Environ()
		deltac.Stdin = strings.NewReader(file.String() + "\n")
		out, err := deltac.Output()
		if err != nil {
			return common.ErrMsg{Err: err}
		}

		return diffContentMsg{cacheKey: key, lines: stringToDiffLines(string(out))}
	}
}

func (m *Model) diffDir(dir *cachedNode, width int, sideBySide bool, preamble string) tea.Cmd {
	if width == 0 || dir == nil {
		return nil
	}
	key := cacheKey(dir.path, sideBySide)
	return func() tea.Msg {
		// Each file gets the args its single-file view would (a new or
		// deleted file is one column, not side-by-side with an empty half),
		// built here for every kind up front: the renders run in parallel.
		argsFor := map[dirFileKind][]string{}
		for _, kind := range []dirFileKind{dirFileChanged, dirFileNew, dirFileDeleted} {
			sbs, opts := kind.deltaLayout(sideBySide)
			argsFor[kind] = m.makeDeltaArgs(sbs, width, opts)
		}
		log.Info("executing delta per file", "cmd", fmt.Sprintf(`delta %s`, strings.Join(argsFor[dirFileChanged], " ")))
		// One delta per file (in parallel) instead of one for the whole
		// directory: it tells where each file starts, which the file tree
		// follows.
		outs, err := renderFiles(dir.files, func(file *gitdiff.File) (string, error) {
			deltac := exec.Command("delta", argsFor[dirFileKindOf(file)]...)
			deltac.Env = os.Environ()
			deltac.Stdin = strings.NewReader(file.String() + "\n")
			out, err := deltac.Output()
			return string(out), err
		})
		if err != nil {
			if stdErr, ok := err.(*exec.ExitError); ok {
				return common.ErrMsg{Err: err, Message: string(stdErr.Stderr)}
			}
			log.Warn("delta error", "err", err)
			return common.ErrMsg{Err: err}
		}

		head := ""
		if preamble != "" {
			head = m.renderPreamble(preamble) + "\n"
		}
		text, starts := joinFileRenders(head, outs)
		paths := make([]string, len(dir.files))
		for i, f := range dir.files {
			paths[i] = filenode.GetFileName(f)
		}
		return diffContentMsg{
			cacheKey:   key,
			lines:      stringToDiffLines(text),
			fileStarts: starts,
			filePaths:  paths,
		}
	}
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

type diffContentMsg struct {
	cacheKey   string
	lines      []diffLine
	fileStarts []int
	filePaths  []string
}

func (m *Model) ClearCache() {
	m.cache = make(nodeCache)
}

func (m *Model) RootDiffStats() (int64, int64) {
	if item, ok := m.cache[cacheKey("/", m.sideBySide)]; ok {
		return item.additions, item.deletions
	}

	return 0, 0
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
// colours to the user's own miasma-dark / miasma-light delta features.
func deltaFeature(theme *tint.Tint) string {
	if !common.IsMiasma(theme) {
		return ""
	}
	if theme.Dark {
		return "miasma-dark"
	}
	return "miasma-light"
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
// layout, plus (unified only) the config that keeps a gitconfig's
// side-by-side = true from overriding the layout asked for.
func (m *Model) makeDeltaArgs(sideBySide bool, width int, opts deltaOpts) []string {
	args := m.themeDeltaArgs(sideBySide, width, opts)
	if sideBySide {
		return args
	}
	return withConfig(args, unifiedGitconfig())
}

// withConfig points delta at the gitconfig file path ("" = the default).
func withConfig(args []string, path string) []string {
	if path == "" {
		return args
	}
	return append(args, "--config="+path)
}

func (m *Model) themeDeltaArgs(sideBySide bool, width int, opts deltaOpts) []string {
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
