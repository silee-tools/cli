package main

import (
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/silee-tools/unid/internal/canvas"
	"github.com/silee-tools/unid/internal/dsl"
	uerr "github.com/silee-tools/unid/internal/errors"
	"github.com/silee-tools/unid/internal/object"
	"github.com/silee-tools/unid/internal/renderer"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:   "unid",
		Short: "Unicode box-drawing diagram renderer",
		Long: `Unicode box-drawing diagram renderer.

A text-based alternative to ASCII diagram editors like Monodraw or ASCIIFlow.
Renders precise Unicode box-drawing diagrams from a simple DSL via stdin.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fi, _ := os.Stdin.Stat()
			if (fi.Mode() & os.ModeCharDevice) != 0 {
				fmt.Fprintln(os.Stderr, "warning: no input provided. Use 'echo \"...\" | unid' or 'unid guide' for details.")
				cmd.Help()
				return nil
			}
			return runRender()
		},
	}

	rootCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List objects in a diagram (stdin)",
		RunE:  func(cmd *cobra.Command, args []string) error { return runList() },
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "lint",
		Short: "Lint DSL input for errors and warnings (stdin)",
		RunE:  func(cmd *cobra.Command, args []string) error { return runLint() },
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "guide",
		Short: "Show comprehensive usage guide with examples",
		Run:   func(cmd *cobra.Command, args []string) { printGuide() },
	})

	var showVersion bool
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Show version")
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if showVersion {
			fmt.Printf("unid v%s © 2026 silee-tools\n", version)
			os.Exit(0)
		}
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func readStdin() (string, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type canvasConfig struct {
	globalOverflow object.ContentOverflow
	globalAlign    object.ContentAlign
	collision      bool
	objects        []object.DrawObject
}

type drawSlot struct {
	obj     object.DrawObject
	pending bool
}

type pendingArrowSlot struct {
	slotIdx int
	srcID   string
	srcSide object.Side
	dstID   string
	dstSide object.Side
	head    rune
	hasHead bool
	both    bool
	legend  *object.Legend
	line    int
}

func processCommands(commands []dsl.DslCommand) (*canvasConfig, error) {
	var globalOverflow object.ContentOverflow
	var globalAlign object.ContentAlign
	var collision *bool
	var globalArrowhead rune
	var hasGlobalArrowhead bool

	var slots []drawSlot
	var arrowSlots []pendingArrowSlot

	for _, cmd := range commands {
		switch c := cmd.(type) {
		case *dsl.CollisionCmd:
			v := c.On
			collision = &v
		case *dsl.OverflowCmd:
			globalOverflow = c.Mode
		case *dsl.AlignCmd:
			globalAlign = c.Mode
		case *dsl.ObjectCmd:
			slots = append(slots, drawSlot{obj: c.Object})
		case *dsl.ArrowCmd:
			idx := len(slots)
			slots = append(slots, drawSlot{pending: true})
			arrowSlots = append(arrowSlots, pendingArrowSlot{
				slotIdx: idx,
				srcID:   c.SrcID, srcSide: c.SrcSide,
				dstID: c.DstID, dstSide: c.DstSide,
				head: c.Head, hasHead: c.HasHead,
				both: c.Both, legend: c.Legend, line: c.Line,
			})
		case *dsl.ArrowheadCmd:
			globalArrowhead = c.Ch
			hasGlobalArrowhead = true
		}
	}

	if collision == nil {
		return nil, &uerr.NoCollisionError{}
	}

	// Resolve arrows
	if err := resolveArrows(slots, arrowSlots, globalArrowhead, hasGlobalArrowhead); err != nil {
		return nil, err
	}

	objects := make([]object.DrawObject, 0, len(slots))
	for _, s := range slots {
		if !s.pending {
			objects = append(objects, s.obj)
		}
	}

	return &canvasConfig{
		globalOverflow: globalOverflow,
		globalAlign:    globalAlign,
		collision:      *collision,
		objects:        objects,
	}, nil
}

func resolveArrows(slots []drawSlot, arrowSlots []pendingArrowSlot, globalArrowhead rune, hasGlobalArrowhead bool) error {
	// Build ID → Anchorable mapping
	idAnchors := map[string]object.Anchorable{}
	for _, s := range slots {
		if s.pending {
			continue
		}
		var id string
		var anchor object.Anchorable
		switch o := s.obj.(type) {
		case *object.Rect:
			id = o.ID
			anchor = o
		case *object.Text:
			id = o.ID
			anchor = o
		case *object.HLine:
			id = o.ID
			anchor = o
		case *object.VLine:
			id = o.ID
			anchor = o
		default:
			continue
		}
		if id == "" {
			continue
		}
		if _, exists := idAnchors[id]; exists {
			return &uerr.ParseError{Line: 0, Message: fmt.Sprintf("duplicate object id '%s'", id)}
		}
		idAnchors[id] = anchor
	}

	for _, as := range arrowSlots {
		src, ok := idAnchors[as.srcID]
		if !ok {
			return &uerr.ParseError{Line: as.line, Message: fmt.Sprintf("unknown object id '%s' in arrow source", as.srcID)}
		}
		dst, ok := idAnchors[as.dstID]
		if !ok {
			return &uerr.ParseError{Line: as.line, Message: fmt.Sprintf("unknown object id '%s' in arrow destination", as.dstID)}
		}

		sx, sy := src.SrcAnchor(as.srcSide)
		ex, ey := dst.DstAnchor(as.dstSide)

		var waypoints [][2]int
		if as.srcID == as.dstID {
			waypoints = object.ComputeSelfLoop(sx, sy, as.srcSide, ex, ey, as.dstSide)
		} else {
			waypoints = object.ComputeRoute(sx, sy, as.srcSide, ex, ey, as.dstSide)
		}

		effectiveHead := as.head
		effectiveHasHead := as.hasHead
		if !effectiveHasHead && hasGlobalArrowhead {
			effectiveHead = globalArrowhead
			effectiveHasHead = true
		}

		slots[as.slotIdx] = drawSlot{
			obj: &object.ResolvedArrow{
				Waypoints: waypoints,
				Head:      effectiveHead,
				HasHead:   effectiveHasHead,
				Both:      as.both,
				Legend:    as.legend,
			},
		}
	}
	return nil
}

func computeCanvasSize(objects []object.DrawObject) (int, int) {
	maxW, maxH := 1, 1
	for _, obj := range objects {
		bw, bh := obj.Bounds()
		if bw > maxW {
			maxW = bw
		}
		if bh > maxH {
			maxH = bh
		}
	}
	return maxW, maxH
}

func runRender() error {
	input, err := readStdin()
	if err != nil {
		return err
	}
	commands, err := dsl.Parse(input)
	if err != nil {
		return err
	}
	config, err := processCommands(commands)
	if err != nil {
		return err
	}
	w, h := computeCanvasSize(config.objects)

	cv := canvas.New(w, h)
	r := renderer.New(cv, config.collision)
	r.GlobalOverflow = config.globalOverflow
	r.GlobalAlign = config.globalAlign

	// Apply global defaults to rects and texts
	objects := make([]object.DrawObject, len(config.objects))
	for i, obj := range config.objects {
		if rect, ok := obj.(*object.Rect); ok {
			if rect.ContentOverflow == object.OverflowEllipsis {
				rect.ContentOverflow = config.globalOverflow
			}
			if rect.ContentAlign == object.AlignLeft {
				rect.ContentAlign = config.globalAlign
			}
		}
		if text, ok := obj.(*object.Text); ok {
			if text.ContentAlign == object.AlignLeft {
				text.ContentAlign = config.globalAlign
			}
		}
		objects[i] = obj
	}

	if err := r.DrawAll(objects); err != nil {
		return err
	}
	fmt.Println(r.Render())
	return nil
}

func runList() error {
	input, err := readStdin()
	if err != nil {
		return err
	}
	commands, err := dsl.Parse(input)
	if err != nil {
		return err
	}
	config, err := processCommands(commands)
	if err != nil {
		return err
	}
	w, h := computeCanvasSize(config.objects)

	fmt.Printf("Canvas: %dx%d (auto)\n", w, h)
	if config.collision {
		fmt.Println("Collision: on")
	} else {
		fmt.Println("Collision: off")
	}
	fmt.Printf("Objects: %d\n", len(config.objects))

	objs := make([]object.DrawObject, len(config.objects))
	copy(objs, config.objects)
	sort.Slice(objs, func(i, j int) bool {
		ac, ar := objs[i].Position()
		bc, br := objs[j].Position()
		if ar != br {
			return ar < br
		}
		return ac < bc
	})

	for i, obj := range objs {
		fmt.Printf("  %d. %s\n", i+1, obj.Summary())
	}
	return nil
}

func runLint() error {
	input, err := readStdin()
	if err != nil {
		return err
	}
	commands, err := dsl.Parse(input)
	if err != nil {
		return err
	}
	config, err := processCommands(commands)
	if err != nil {
		return err
	}
	w, h := computeCanvasSize(config.objects)

	fmt.Printf("Canvas: %dx%d\n", w, h)
	if config.collision {
		fmt.Println("Collision: on")
	} else {
		fmt.Println("Collision: off")
	}
	fmt.Printf("Objects: %d\n", len(config.objects))

	var warnings, errors []string

	for i, obj := range config.objects {
		bw, bh := obj.Bounds()
		if bw > w || bh > h {
			msg := fmt.Sprintf("object #%d (%s): bounds (%dx%d) exceed canvas (%dx%d)",
				i+1, obj.CollisionDesc(), bw, bh, w, h)
			if config.collision {
				errors = append(errors, msg)
			} else {
				warnings = append(warnings, msg)
			}
		}
	}

	if config.collision {
		cv := canvas.New(w, h)
		r := renderer.New(cv, true)
		for _, obj := range config.objects {
			if err := r.Draw(obj); err != nil {
				errors = append(errors, err.Error())
			}
		}
	}

	if len(warnings) > 0 {
		fmt.Println("Warnings:")
		for _, w := range warnings {
			fmt.Printf("  - %s\n", w)
		}
	}
	if len(errors) > 0 {
		fmt.Println("Errors:")
		for _, e := range errors {
			fmt.Printf("  - %s\n", e)
		}
		os.Exit(1)
	}

	if len(warnings) == 0 && len(errors) == 0 {
		fmt.Println("OK")
	}
	return nil
}
