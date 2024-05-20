package main

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/jawher/mow.cli"
	"github.com/joshuarubin/go-sway"
	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("xsway")

var ErrWorkspaceNotFound = errors.New("No workspace found")
var ErrArgParseFailed = errors.New("Failed to parse args")

type Direction string
type Orientation string
type Layout string

const (
	Left       = Direction("left")
	Right      = Direction("right")
	Up         = Direction("up")
	Down       = Direction("down")
	Horizontal = Orientation("horizontal")
	Vertical   = Orientation("vertical")
	Default    = Layout("default")
	Tabbed     = Layout("tabbed")
	Stacking   = Layout("stacking")
)

func inverse(d Direction) Direction {
	switch d {
	case Left:
		return Right
	case Right:
		return Left
	case Up:
		return Down
	case Down:
		return Up
	}
	return Direction("")
}

func getStdin(cliArgs []string) []string {
	fi, _ := os.Stdin.Stat()
	if fi.Mode()&os.ModeNamedPipe != 0 {
		reader := bufio.NewReader(os.Stdin)
		res, err := reader.ReadString('\n')
		if err != nil {
			return cliArgs
		}
		res = strings.TrimSpace(res)
		for _, arg := range strings.Split(res, " ") {
			cliArgs = append(cliArgs, arg)
		}
	}
	return cliArgs
}

func main() {
	xsway := cli.App("xsway", "XMonad workspace handling and more for sway-wm")
	xsway.Version("v version", "0.1")

	ctx := context.Background()

	debug := xsway.Bool(cli.BoolOpt{
		Name:  "debug",
		Value: false,
		Desc:  "Enable debug logs",
	})

	xsway.Before = func() {
		if *debug {
			backend := logging.NewLogBackend(os.Stderr, "", 0)
			backendLevel := logging.AddModuleLevel(backend)
			backendLevel.SetLevel(logging.DEBUG, "")
			logging.SetBackend(backend)
		}
	}

	xsway.Command("focus-output-horizontal-position", "Change focusd output by horizontal position", func(cmd *cli.Cmd) {
		position := cmd.StringArg("POSITION", "", "Position name")

		cmd.Action = func() {
			FocusOutputHorizontalPosition(ctx, *position)
		}
	})

	xsway.Command("show", "Show or create workspace on focused screen", func(cmd *cli.Cmd) {
		wsName := cmd.StringArg("WSNAME", "", "Workspace name")

		cmd.Action = func() {
			Show(ctx, *wsName)
		}
	})

	xsway.Command("rename", "Rename current workspace", func(cmd *cli.Cmd) {
		wsName := cmd.StringArg("WSNAME", "", "New workspace name")

		cmd.Action = func() {
			Rename(ctx, *wsName)
		}
	})

	xsway.Command("bind", "Bind current workspace to num", func(cmd *cli.Cmd) {
		num := cmd.StringArg("NUM", "", "")

		cmd.Action = func() {
			Bind(ctx, *num)
		}
	})

	xsway.Command("swap", "Swap visible workspaces when there is 2 screens", func(cmd *cli.Cmd) {
		cmd.Action = func() {
			Swap(ctx)
		}
	})

	xsway.Command("list", "List all workspace names", func(cmd *cli.Cmd) {
		cmd.Action = func() {
			List(ctx)
		}
	})

	xsway.Command("current", "Current workspace name", func(cmd *cli.Cmd) {
		cmd.Action = func() {
			Current(ctx)
		}
	})

	xsway.Command("move", "Move current container to workspace", func(cmd *cli.Cmd) {
		ws := cmd.StringArg("NUM_OR_NAME", "", "")

		cmd.Action = func() {
			Move(ctx, *ws)
		}
	})

	xsway.Command("merge", "Merge current container into other container", func(cmd *cli.Cmd) {
		d := cmd.StringArg("DIRECTION", "", "The direction where to merge (left/right/up/down)")
		o := cmd.StringArg("ORIENTATION", "", "Split mode (horizontal/vertical)")
		l := cmd.StringArg("LAYOUT", "", "Layout type to use (default/tabbed/stacking)")

		cmd.Action = func() {
			Merge(ctx, Direction(*d), Orientation(*o), Layout(*l))
		}
	})

	args := getStdin(os.Args)
	xsway.Run(args)
}

// WS represents the set of shared workspaces
type WS []sway.Workspace

func (ws WS) Len() int {
	return len(ws)
}

func (ws WS) Swap(i, j int) {
	ws[i], ws[j] = ws[j], ws[i]
}

func (ws WS) Less(i, j int) bool {
	if ws[i].Num != -1 {
		return ws[i].Num < ws[j].Num
	} else {
		return ws[i].Name < ws[j].Name
	}
}

type XSway struct {
	client     sway.Client
	workspaces []sway.Workspace
	outputs    []sway.Output
	chain      *CmdChain
}

func (c *XSway) RunChain(ctx context.Context) {
	cmd := strings.Join(*c.chain, ";")
	log.Debugf("Run: %s", cmd)
	c.client.RunCommand(ctx, cmd)
}

func (c *XSway) GetWSNum(ctx context.Context, num int64) (sway.Workspace, error) {
	for _, ws := range c.workspaces {
		if ws.Num == num {
			return ws, nil
		}
	}
	return sway.Workspace{}, ErrWorkspaceNotFound
}

func (c *XSway) GetWSName(ctx context.Context, name string) (sway.Workspace, error) {
	for _, ws := range c.workspaces {
		if strings.Contains(ws.Name, name) {
			return ws, nil
		}
	}
	return sway.Workspace{}, ErrWorkspaceNotFound
}

func (c *XSway) GetWS(ctx context.Context, nameOrNum string) (sway.Workspace, error) {
	var ws sway.Workspace
	num, err := strconv.ParseInt(nameOrNum, 10, 32)
	if err != nil {
		ws, err = c.GetWSName(ctx, nameOrNum)
	} else {
		ws, err = c.GetWSNum(ctx, num)
	}

	if err != nil {
		return sway.Workspace{}, err
	}

	return ws, nil
}

func (c *XSway) CurrentWS() (sway.Workspace, error) {
	for _, ws := range c.workspaces {
		if ws.Focused == true {
			return ws, nil
		}
	}
	return sway.Workspace{}, ErrWorkspaceNotFound
}

func (c *XSway) OutputWS(ctx context.Context, output string) (sway.Workspace, error) {
	for _, ws := range c.workspaces {
		if ws.Visible && ws.Output == output {
			return ws, nil
		}
	}
	return sway.Workspace{}, ErrWorkspaceNotFound
}

func (c *XSway) ActiveOutputs(ctx context.Context) ([]sway.Output, error) {
	var active []sway.Output
	outputs, err := c.client.GetOutputs(ctx)
	if err == nil {
		for _, o := range outputs {
			if o.Active {
				active = append(active, o)
			}
		}
	}
	return active, err
}

type CmdChain []string

func (c *CmdChain) Add(cmd string) {
	log.Debugf("Add cmd: %s", cmd)
	*c = append(*c, cmd)
}

func (c *CmdChain) ShowWS(ws sway.Workspace) {
	c.Add("workspace " + string(ws.Name))
}

func (c *CmdChain) RenameWS(wsName string) {
	c.Add("rename workspace to " + wsName)
}

func (c *CmdChain) MoveWSToOuput(output string) {
	c.Add("move workspace to output " + output)
}

func (c *CmdChain) FocusOutput(output string) {
	c.Add("focus output " + output)
}

func (c *CmdChain) SwapWS(ws1 sway.Workspace, ws2 sway.Workspace) {
	c.MoveWSToOuput(ws2.Output)
	c.ShowWS(ws2)
	c.MoveWSToOuput(ws1.Output)
	c.FocusOutput(ws1.Output)
}

func (c *CmdChain) ShowWSOnOutput(ws sway.Workspace, output string) {
	c.ShowWS(ws)
	if ws.Output != output {
		c.MoveWSToOuput(output)
	}
}

func (c *CmdChain) MoveContainerToWS(wsName string) {
	c.Add("move container to workspace " + wsName)
}

func (c *CmdChain) FocusContainer(d Direction) {
	c.Add(fmt.Sprintf("focus %s", d))
}

func (c *CmdChain) SplitContainer(o Orientation) {
	c.Add(fmt.Sprintf("split %s", o))
}

func (c *CmdChain) MoveContainer(d Direction) {
	c.Add(fmt.Sprintf("move %s", d))
}

func (c *CmdChain) ChangeLayout(l Layout) {
	c.Add(fmt.Sprintf("layout %s", l))
}

func WSName(ws sway.Workspace) string {
	var name string
	splitName := strings.Split(ws.Name, ":")
	if len(splitName) > 1 {
		name = splitName[1]
	} else {
		name = ws.Name
	}
	return name
}

func Init(ctx context.Context) XSway {
	client, _ := sway.New(ctx)
	workspaces, _ := client.GetWorkspaces(ctx)
	outputs, _ := client.GetOutputs(ctx)
	chain := CmdChain{}
	return XSway{
		client:     client,
		workspaces: workspaces,
		outputs:    outputs,
		chain:      &chain,
	}
}

func FocusOutputHorizontalPosition(ctx context.Context, position string) {
	c := Init(ctx)
	intPos, err := strconv.ParseInt(position, 10, 64)
	if err != nil {
		return
	}
	if len(c.outputs) <= int(intPos) {
		return
	}
	lenCmp := func(a, b sway.Output) int {
        return cmp.Compare(a.Rect.X, b.Rect.X)
    }
	slices.SortFunc(c.outputs, lenCmp)
	c.chain.FocusOutput(c.outputs[intPos].Name)
	c.RunChain(ctx)
}

func Show(ctx context.Context, wsName string) {
	c := Init(ctx)

	targetWS, err := c.GetWS(ctx, wsName)
	if err != nil {
		c.chain.ShowWS(sway.Workspace{Name: wsName})
		c.RunChain(ctx)
		return
	}

	currentWS, _ := c.CurrentWS()
	if currentWS == targetWS {
		return
	}

	if currentWS.Visible && targetWS.Visible {
		c.chain.SwapWS(currentWS, targetWS)
	} else {
		// bring workspace to output
		c.chain.ShowWSOnOutput(targetWS, currentWS.Output)
		// make WS history correct
		c.chain.ShowWS(currentWS)
		c.chain.ShowWS(targetWS)
	}
	c.chain.FocusOutput(currentWS.Output)
	c.RunChain(ctx)
}

func Swap(ctx context.Context) {
	c := Init(ctx)
	outputs, _ := c.ActiveOutputs(ctx)
	if len(outputs) != 2 {
		return
	}
	ws1, _ := c.GetWS(ctx, outputs[0].CurrentWorkspace)
	ws2, _ := c.GetWS(ctx, outputs[1].CurrentWorkspace)
	if ws1.Focused {
		c.chain.SwapWS(ws1, ws2)
	} else {
		c.chain.SwapWS(ws2, ws1)
	}
	c.RunChain(ctx)
}

func List(ctx context.Context) {
	c := Init(ctx)
	names := make([]string, len(c.workspaces))
	sort.Sort(WS(c.workspaces))
	for i, ws := range c.workspaces {
		names[i] = ws.Name
	}
	fmt.Printf(strings.Join(names, "\n"))
}

func Current(ctx context.Context) {
	c := Init(ctx)
	ws, _ := c.CurrentWS()
	fmt.Printf(WSName(ws))
}

func Rename(ctx context.Context, wsName string) {
	c := Init(ctx)

	ws, _ := c.CurrentWS()
	if ws.Num != -1 {
		wsName = strconv.Itoa(int(ws.Num)) + ":" + wsName
	}

	c.chain.RenameWS(wsName)
	c.RunChain(ctx)
}

func Bind(ctx context.Context, wsNum string) {
	c := Init(ctx)

	currentWS, _ := c.CurrentWS()
	currentName := WSName(currentWS)
	currentNum := strconv.Itoa(int(currentWS.Num))
	if currentNum == wsNum {
		return
	}

	otherWS, err := c.GetWS(ctx, wsNum)
	if err == nil {
		otherName := WSName(otherWS)
		c.chain.ShowWS(otherWS)
		// num to bind for the other WS
		if currentNum != "-1" {
			c.chain.RenameWS(currentNum + ":" + otherName)
		} else {
			c.chain.RenameWS(otherName)
		}
		// restore other output WS if needed
		if otherWS.Output != currentWS.Output {
			otherOutputWS, err := c.OutputWS(ctx, otherWS.Output)
			if err == nil && otherOutputWS != otherWS {
				c.chain.ShowWS(otherOutputWS)
			}
		}
		c.chain.ShowWS(currentWS)
	} else {
		fmt.Printf("%s\n", err)
	}

	c.chain.RenameWS(wsNum + ":" + currentName)
	c.chain.FocusOutput(currentWS.Output)
	c.RunChain(ctx)
}

func Move(ctx context.Context, wsName string) {
	c := Init(ctx)
	ws, err := c.GetWS(ctx, wsName)
	// new workspace
	if err != nil {
		c.chain.MoveContainerToWS(wsName)
	} else {
		c.chain.MoveContainerToWS(ws.Name)
	}
	c.RunChain(ctx)
}

func Merge(ctx context.Context, d Direction, o Orientation, l Layout) {
	c := Init(ctx)
	c.chain.FocusContainer(d)
	c.chain.SplitContainer(o)
	c.chain.FocusContainer(inverse(d))
	c.chain.MoveContainer(d)
	c.chain.ChangeLayout(l)
	c.RunChain(ctx)
}
