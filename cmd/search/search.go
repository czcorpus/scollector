package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/czcorpus/scollector/storage"
)

func main() {

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "search - search for collocations of a provided lemma\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n\t%s [options] [db_path] [lemma]\n\t", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()
	db, err := storage.OpenDB(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR: ", err)
		os.Exit(1)
	}
	ans, err := db.CalculateLogDice(flag.Arg(1))
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR: ", err)
		os.Exit(1)
	}
	for _, item := range ans {
		fmt.Println(item.TabString())
	}
}
