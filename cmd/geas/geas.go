// Copyright 2023 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/fjl/geas/asm"
	"github.com/fjl/geas/disasm"
	"github.com/fjl/geas/internal/evm"
)

var t2s = strings.NewReplacer("\t", "  ")

func usage() {
	fmt.Fprint(os.Stderr, t2s.Replace(`
Usage: geas {-a | -d | -i} [options...] <file>

 -a: ASSEMBLER (default)

	 -o <file>          output file name
	 -bin               output binary instead of hex
	 -no-nl             skip newline at end of hex output

 -d: DISASSEMBLER

	 -bin               input is binary bytecode
	 -target <name>     configure instruction set
	 -o <file>          output file name
	 -blocks            blank lines between logical blocks
	 -pc                show program counter
	 -uppercase         show instruction names as uppercase

 -i: INFORMATION

	 -targets           show supported target fork names
	 -ops <target>      show all opcodes in target
	 -lineage <target>  show target fork chain

 -h: HELP

`))
}

type config struct {
	Binary bool
	NoNL   bool
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	mode := os.Args[1]
	switch {
	case mode == "-a":
		assembler(os.Args[2:])

	case mode == "-d":
		disassembler(os.Args[2:])

	case mode == "-i":
		information(os.Args[2:])

	case mode == "-h", mode == "-help", mode == "--help":
		usage()
		os.Exit(0)

	default:
		assembler(os.Args[1:])
	}
}

const inputLimit = 10 * 1024 * 1024

func assembler(args []string) {
	var (
		fs         = newFlagSet("-a")
		outputFile = fs.String("o", "", "")
		binary     = fs.Bool("bin", false, "")
		noNL       = fs.Bool("no-nl", false, "")
	)
	parseFlags(fs, args)

	// Assemble.
	var c = asm.New(nil)
	var bin []byte
	file := fileArg(fs)
	if file != "-" {
		wd, _ := os.Getwd()
		c.SetFilesystem(os.DirFS(wd))
		fp := path.Clean(filepath.ToSlash(file))
		bin = c.CompileFile(fp)
	} else {
		source, err := io.ReadAll(io.LimitReader(os.Stdin, inputLimit))
		if err != nil {
			exit(1, err)
		}
		bin = c.CompileString(string(source))
	}

	// Show errors.
	for _, err := range c.ErrorsAndWarnings() {
		fmt.Fprintln(os.Stderr, err)
	}
	if c.Failed() {
		os.Exit(1)
	}

	// Write output.
	var err error
	output := os.Stdout
	if *outputFile != "" {
		output, err = os.OpenFile(*outputFile, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
		if err != nil {
			exit(1, err)
		}
		defer output.Close()
	}
	if *binary {
		_, err = output.Write(bin)
	} else {
		nl := "\n"
		if *noNL {
			nl = ""
		}
		_, err = fmt.Fprintf(output, "%x%s", bin, nl)
	}
	if err != nil {
		exit(1, err)
	}
}

func disassembler(args []string) {
	var (
		fs         = newFlagSet("-d")
		outputFile = fs.String("o", "", "")
		showPC     = fs.Bool("pc", false, "")
		showBlocks = fs.Bool("blocks", true, "")
		uppercase  = fs.Bool("uppercase", false, "")
		binary     = fs.Bool("bin", false, "")
		target     = fs.String("target", "", "")
	)
	parseFlags(fs, args)

	// Read input.
	var err error
	var infd io.ReadCloser
	file := fileArg(fs)
	if file == "-" {
		infd = os.Stdin
	} else {
		infd, err = os.Open(file)
		if err != nil {
			exit(1, err)
		}
	}
	bytecode, err := io.ReadAll(io.LimitReader(infd, inputLimit))
	if err != nil {
		exit(1, err)
	}
	infd.Close()

	// Possibly convert from hex.
	if !*binary {
		dec := make([]byte, hex.DecodedLen(len(bytecode)))
		l, err := hex.Decode(dec, bytes.TrimSpace(bytecode))
		if err != nil {
			exit(1, err)
		}
		bytecode = dec[:l]
	}

	output := os.Stdout
	if *outputFile != "" {
		output, err = os.OpenFile(*outputFile, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
		if err != nil {
			exit(1, err)
		}
		defer output.Close()
	}

	// Disassemble.
	d := disasm.New()
	d.SetShowBlocks(*showBlocks)
	d.SetShowPC(*showPC)
	d.SetUppercase(*uppercase)
	if *target != "" {
		if err := d.SetTarget(*target); err != nil {
			exit(2, err)
		}
	}
	err = d.Disassemble(bytecode, output)
	exit(1, err)
}

func information(args []string) {
	var ran bool
	checkRunOnce := func() {
		if ran {
			exit(2, fmt.Errorf("can't show more than one thing at once in -i mode"))
		}
		ran = true
	}
	showTargets := func(arg string) error {
		checkRunOnce()
		for _, name := range evm.AllForks() {
			fmt.Println(name)
		}
		return nil
	}
	showOps := func(arg string) error {
		checkRunOnce()
		is := evm.FindInstructionSet(arg)
		if is == nil {
			return fmt.Errorf("Error: unknown fork %q", flag.Arg(0))
		}
		for _, op := range is.AllOps() {
			fmt.Println(op.Name)
		}
		return nil
	}
	showParents := func(arg string) error {
		checkRunOnce()
		is := evm.FindInstructionSet(arg)
		if is == nil {
			return fmt.Errorf("Error: unknown fork %q", flag.Arg(0))
		}
		for _, f := range is.Parents() {
			fmt.Println(f)
		}
		return nil
	}

	var fs = newFlagSet("-i")
	fs.BoolFunc("targets", "", showTargets)
	fs.Func("ops", "", showOps)
	fs.Func("lineage", "", showParents)
	parseFlags(fs, args)
	if !ran {
		usage()
		exit(2, fmt.Errorf("please select information topic"))
	}
	if fs.NArg() > 0 {
		exit(2, fmt.Errorf("too many arguments"))
	}
}

func newFlagSet(mode string) *flag.FlagSet {
	fs := flag.NewFlagSet("geas "+mode, flag.ContinueOnError)
	fs.Usage = usage
	fs.SetOutput(io.Discard)
	return fs
}

func parseFlags(fs *flag.FlagSet, args []string) {
	if err := fs.Parse(args); err != nil {
		exit(2, err)
	}
}

func fileArg(fs *flag.FlagSet) string {
	switch fs.NArg() {
	case 1:
		return fs.Arg(0)
	case 0:
		exit(2, fmt.Errorf("need file name as argument"))
	default:
		exit(2, fmt.Errorf("too many arguments"))
	}
	return ""
}

func exit(code int, err error) {
	if err == nil || err == flag.ErrHelp {
		os.Exit(0)
	}
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(code)
}
