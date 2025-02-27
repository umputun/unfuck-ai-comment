package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	// Define command line flags
	outputMode := flag.String("output", "inplace", "Output mode: inplace, print, diff")
	dryRun := flag.Bool("dry-run", false, "Don't modify files, just show what would be changed")
	showHelp := flag.Bool("help", false, "Show usage information")
	flag.Parse()
	
	// If dry-run is set, override output mode to diff
	if *dryRun {
		*outputMode = "diff"
	}

	// Show help if requested
	if *showHelp {
		fmt.Println("unfuck-ai-comments - Convert in-function comments to lowercase")
		fmt.Println("\nUsage:")
		fmt.Println("  unfuck-ai-comments [options] [file/pattern...]")
		fmt.Println("\nOptions:")
		flag.PrintDefaults()
		fmt.Println("\nExamples:")
		fmt.Println("  unfuck-ai-comments                       # Process all .go files in current directory")
		fmt.Println("  unfuck-ai-comments file.go               # Process specific file")
		fmt.Println("  unfuck-ai-comments ./...                 # Process all .go files recursively")
		fmt.Println("  unfuck-ai-comments -output=print file.go # Print modified file to stdout")
		fmt.Println("  unfuck-ai-comments -output=diff *.go     # Show diff for all .go files")
		return
	}

	// Get files to process
	var patterns []string
	if flag.NArg() > 0 {
		patterns = flag.Args()
	} else {
		patterns = []string{"."}
	}

	// Process each pattern
	for _, pattern := range patterns {
		processPattern(pattern, *outputMode)
	}
}

func processPattern(pattern, outputMode string) {
	// Handle special "./..." pattern for recursive search
	if pattern == "./..." {
		walkDir(".", outputMode)
		return
	}

	// If it's a recursive pattern, handle it
	if strings.HasSuffix(pattern, "/...") || strings.HasSuffix(pattern, "...") {
		// Extract the directory part
		dir := strings.TrimSuffix(pattern, "/...")
		dir = strings.TrimSuffix(dir, "...")
		if dir == "" {
			dir = "."
		}
		walkDir(dir, outputMode)
		return
	}

	// Find all .go files matching the pattern
	files, err := filepath.Glob(pattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error globbing pattern %s: %v\n", pattern, err)
		return
	}

	// If pattern isn't a glob pattern, try as directory
	if len(files) == 0 {
		// Remove any trailing slash for consistency
		cleanPattern := strings.TrimSuffix(pattern, "/")
		
		fileInfo, err := os.Stat(cleanPattern)
		if err == nil && fileInfo.IsDir() {
			// Process all go files in the directory
			matches, _ := filepath.Glob(filepath.Join(cleanPattern, "*.go"))
			if len(matches) > 0 {
				files = append(files, matches...)
			}
		}
	}

	// Process each file
	for _, file := range files {
		if !strings.HasSuffix(file, ".go") {
			continue
		}
		processFile(file, outputMode)
	}
}

// walkDir recursively processes all .go files in directory and subdirectories
func walkDir(dir, outputMode string) {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			processFile(path, outputMode)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error walking directory %s: %v\n", dir, err)
	}
}

func processFile(fileName, outputMode string) {
	// Parse the file
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, fileName, nil, parser.ParseComments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", fileName, err)
		return
	}

	// Process comments
	modified := false
	for _, commentGroup := range node.Comments {
		for _, comment := range commentGroup.List {
			// Check if comment is inside a function
			if isCommentInsideFunction(fset, node, comment) {
				// Process the comment text
				orig := comment.Text
				lower := convertCommentToLowercase(orig)
				if orig != lower {
					comment.Text = lower
					modified = true
				}
			}
		}
	}

	// If no comments were modified, no need to proceed
	if !modified {
		return
	}

	// Handle output based on specified mode
	switch outputMode {
	case "inplace":
		// Write modified source back to file
		file, err := os.Create(fileName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening %s for writing: %v\n", fileName, err)
			return
		}
		defer file.Close()
		if err := printer.Fprint(file, fset, node); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing to file %s: %v\n", fileName, err)
				return
			}
		fmt.Printf("Updated: %s\n", fileName)

	case "print":
		// Print modified source to stdout
		if err := printer.Fprint(os.Stdout, fset, node); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing to stdout: %v\n", err)
				return
			}

	case "diff":
		// Generate diff output
		origBytes, err := os.ReadFile(fileName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading original file %s: %v\n", fileName, err)
			return
		}

		var modifiedBytes strings.Builder
		if err := printer.Fprint(&modifiedBytes, fset, node); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating diff: %v\n", err)
				return
			}

		fmt.Printf("--- %s (original)\n", fileName)
		fmt.Printf("+++ %s (modified)\n", fileName)
		// Very basic diff output - in a real implementation, use a proper diff library
		fmt.Println(simpleDiff(string(origBytes), modifiedBytes.String()))
	}
}

// isCommentInsideFunction checks if a comment is inside a function declaration
func isCommentInsideFunction(_ *token.FileSet, file *ast.File, comment *ast.Comment) bool {
	commentPos := comment.Pos()
	
	// Find function containing the comment
	var insideFunc bool
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return true
		}

		// Check if this is a function declaration
		fn, isFunc := n.(*ast.FuncDecl)
		if isFunc {
			// Check if comment is inside function body
			if fn.Body != nil && fn.Body.Lbrace <= commentPos && commentPos <= fn.Body.Rbrace {
				insideFunc = true
				return false // Stop traversal
			}
		}
		return true
	})

	return insideFunc
}

// convertCommentToLowercase converts a comment to lowercase, preserving the comment markers
func convertCommentToLowercase(comment string) string {
	if strings.HasPrefix(comment, "//") {
		// Single line comment
		content := strings.TrimPrefix(comment, "//")
		return "//" + strings.ToLower(content)
	} else if strings.HasPrefix(comment, "/*") && strings.HasSuffix(comment, "*/") {
		// Multi-line comment
		content := strings.TrimSuffix(strings.TrimPrefix(comment, "/*"), "*/")
		return "/*" + strings.ToLower(content) + "*/"
	}
	return comment
}

// simpleDiff creates a very basic diff output
func simpleDiff(original, modified string) string {
	origLines := strings.Split(original, "\n")
	modLines := strings.Split(modified, "\n")
	
	var diff strings.Builder
	
	for i := 0; i < len(origLines) || i < len(modLines); i++ {
		switch {
		case i >= len(origLines):
			diff.WriteString("+ " + modLines[i] + "\n")
		case i >= len(modLines):
			diff.WriteString("- " + origLines[i] + "\n")
		case origLines[i] != modLines[i]:
			diff.WriteString("- " + origLines[i] + "\n")
			diff.WriteString("+ " + modLines[i] + "\n")
		}
	}
	
	return diff.String()
}