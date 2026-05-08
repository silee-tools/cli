package dsl

import "github.com/silee-tools/unid/internal/object"

type DslCommand interface {
	isDslCommand()
}

type CollisionCmd struct {
	On bool
}

func (c *CollisionCmd) isDslCommand() {}

type OverflowCmd struct {
	Mode object.ContentOverflow
}

func (c *OverflowCmd) isDslCommand() {}

type AlignCmd struct {
	Mode object.ContentAlign
}

func (c *AlignCmd) isDslCommand() {}

type ObjectCmd struct {
	Object object.DrawObject
}

func (c *ObjectCmd) isDslCommand() {}

type ArrowCmd struct {
	SrcID, DstID     string
	SrcSide, DstSide object.Side
	Head             rune
	HasHead          bool
	Both             bool
	Legend           *object.Legend
	Line             int
}

func (c *ArrowCmd) isDslCommand() {}

type ArrowheadCmd struct {
	Ch rune
}

func (c *ArrowheadCmd) isDslCommand() {}
