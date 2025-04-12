package main

import "path/filepath"
import "strings"
import "os/exec"
import "regexp"
import "fmt"
import "bufio"
import "flag"
import "os"

func transpileCgoFile(filePath string, outputFile string, overwrite bool, run bool, compileOnly bool, debug bool, args []string) {
    fileName := strings.TrimSuffix(filePath, filepath.Ext(filePath))
     var tempDir string
    if os.Getenv("OS") == "Windows_NT" {
        tempDir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local", "Temp")
    } else {
        tempDir = "/tmp/"
    }
    goFilePath := filepath.Join(tempDir, filepath.Base(fileName) + ".go")
    if _, err := os.Stat(goFilePath); err == nil && !overwrite {
        fmt.Printf("File %s already exists. Overwrite? [y/n] ", goFilePath)
         var response string
        fmt.Scan(&response)
        if strings.ToLower(response) != "y" {
            fmt.Println("Aborting.")
            os.Exit(1)
        }
    }
    inputBytes, err := os.ReadFile(filePath)
    if err != nil {
        fmt.Println("Error reading file: %s", err)
        os.Exit(1)
    }
    code := string(inputBytes)
    replacements := [][2]string{
        {`#use stdio;`, `import "fmt"`},
        {`#use (.*);`, `import "$1"`},
        {`mov\((.*)\)`, `copy($1);`},
        {`std.outln\((.*)\);`, `fmt.Println($1)`},
        {`std.sout\((.*)\);`, `fmt.Sprintf($1)`},
        {`std.out\((.*)\);`, `fmt.Printf($1)`},
        {`std.in\((.*)\);`, `fmt.Scan($1)`},
        {`std.err\((.*)\);`, `fmt.Errorf($1)`},
        {`ret\((.*?)\);`, `return $1`},
        {`void main\((.*?)\) \[\];`, `func main($1)`},
        {`void main\((.*?)\);`, `func main($1)`},
        {`const any (.*) = (.*);`, `const $1 = $2`},
        {`const (.*) (.*) = (.*);`, `const $2 $1 = $3`},
        {`any (.*) = Null`, `var $1 interface{}`},
        {`(\s*)(\S+) (\S+) = Null`, `$1 var $3 $2`},
        {`any (.*) = (.*)`, `$1 := $2`},
        {`\((.*) (.*) = (.*)`, `($2 := $3`},
        {`(\s*)([^,\s]+) (?:, )?(\S+) = (.*)`, `$1 var $3 $2 = $4`},
        {`struct \(`, `var (`},
        {`struct > (.*) \(`, `type $1 struct {`},
        {`func (.*)\((.*?)\) \[(.*)\];`, `func $1($2) $3`},
        {`func (.*)\((.*?)\) \[\];`, `func $1($2)`},
        {`func (.*)\((.*?)\);`, `func $1($2)`},
        {`func\((.*?)\) \[(.*)\]`, `func($1) $2`},
        {`func\((.*?)\) \[\]`, `func($1)`},
        {`func\((.*?)\)`, `func($1)`},
        {`elif \((.*)\);`, `else if $1`},
        {`if \((.*)\);`, `if $1`},
        {`loop \((.*)\);`, `for $1`},
        {`(.*) >> (.*)`, `$1 > $2`},
        {`(.*) << (.*)`, `$1 < $2`},
        {`range\((.*)\)`, `range $1`},
        {`;$n?`, `$n`},
        {`Null`, `nil`},
    }
    lns := strings.Split(code, "\n")
    for i, ln := range lns {
        ln = strings.TrimSpace(ln)
        if ln != "" && !strings.HasSuffix(ln, ";") && !strings.HasSuffix(ln, "{") && !strings.HasSuffix(ln, "(") && !strings.HasSuffix(ln, ",") && !strings.HasSuffix(ln, ":") && !strings.HasPrefix(ln, "//") {
            fmt.Println("Error on line", i+1, ": Missing semicolon at the end of the line.")
            os.Exit(1)
        }
    }
    scanner := bufio.NewScanner(strings.NewReader(code))
     var lines []string
    lineNumber := 1
    for scanner.Scan() {
        line := scanner.Text()
        newLine := replaceOutsideQuotes(line, replacements, goFilePath, lineNumber)
        lines = append(lines, newLine)
        lineNumber++
    }
    output := "package main\n\n" + strings.Join(lines, "\n")
    err = os.WriteFile(goFilePath, []byte(output), 0644)
    if err != nil {
        fmt.Println("Error writing Go file:", err)
        os.Exit(1)
    }
    if debug == true {
        gFP := filepath.Join(".", filepath.Base(fileName) + ".go")
        debugOutput := "package main\n\n" + strings.Join(lines, "\n")
        debugErr := os.WriteFile(gFP, []byte(debugOutput), 0644)
        if debugErr != nil {
            fmt.Println("Error writing Go file:", debugErr)
            os.Exit(1)
        }
    }
    if compileOnly {
        cmd := exec.Command("go", "build", "-o", outputFile, goFilePath)
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr
        cmd.Run()
        os.Remove(goFilePath)
    } else if run {
        cmdArgs := []string{"run", goFilePath}
        if len(args) > 0 {
            cmdArgs = append(cmdArgs, args...)
        }
        cmd := exec.Command("go", cmdArgs...)
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr
        cmd.Run()
        os.Remove(goFilePath)
    }
}

func replaceOutsideQuotes(line string, replacements [][2]string, goFilePath string, lineNumber int) string {
    quotes := []string{}
    quoteRegex := regexp.MustCompile(`"[^"]*"|'[^']*'|` + "`[^`]*`")
    processedLine := quoteRegex.ReplaceAllStringFunc(line, func(match string) string {
        placeholder := fmt.Sprintf("[%d]", len(quotes))
        quotes = append(quotes, match)
        return placeholder
    })
    if regexp.MustCompile(`func main \(.*\);`).MatchString(processedLine) {
        fmt.Println("The main function on line %d must be declared as void main(<args>);", lineNumber)
        os.Remove(goFilePath)
        os.Exit(1)
    }
    for _, pair := range replacements {
        re := regexp.MustCompile(pair[0])
        processedLine = re.ReplaceAllString(processedLine, pair[1])
    }
    for i, quote := range quotes {
        placeholder := fmt.Sprintf("[%d]", i)
        processedLine = strings.Replace(processedLine, placeholder, quote, 1)
    }
    return processedLine
}

func insideQuotes(line string) bool {
    tokens := shlexSplit(line)
    for _, token := range tokens {
        if strings.HasPrefix(token, "\"") || strings.HasPrefix(token, "'") || strings.HasPrefix(token, "`") {
            return true
        }
    }
    return false
}

func shlexSplit(line string) []string {
     var tokens []string
     var token string
     var inQuotes bool = false
    for i := 0; i < len(line); i++ {
        c := line[i]
        if (c == '"' || c == '\'') && !inQuotes {
            inQuotes = true
            token += string(c)
        } else if (c == '"' || c == '\'') && inQuotes {
            inQuotes = false
            token += string(c)
            tokens = append(tokens, token)
            token = ""
        } else if i+1 < len(line) && line[i] == '/' && line[i+1] == '/' && !inQuotes {
            break
        } else if c == ' ' && !inQuotes {
            if token != "" {
                tokens = append(tokens, token)
                token = ""
            }
        } else {
            token += string(c)
        }
    }
    if token != "" {
        tokens = append(tokens, token)
    }
    return tokens
}

func main() {
    filePath := flag.String("file", "", "Path to the .hyp file")
    compileOnly := flag.Bool("compile", false, "Compile the .hyp file")
    run := flag.Bool("run", false, "Run the .hyp file")
    overwrite := flag.Bool("overwrite", false, "Overwrite the output file")
    debug := flag.Bool("debug", false, "It generates the go file in the current directory")
    outputFile := flag.String("out", "", "Output executable file")
    flag.Parse()
    if *filePath == "" {
        fmt.Println("Please provide a .cgo file with -file <file>")
        os.Exit(1)
    }
    args := flag.Args()
    transpileCgoFile(*filePath, *outputFile, *overwrite, *run, *compileOnly, *debug, args)
}