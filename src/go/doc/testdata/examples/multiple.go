// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package foo_test

import (
	"flag"
	"fmt"
	"log"
	"os/exec"
	"sort"
)

func ExampleHello() {
	fmt.Println("Hello, world!")
	// Output: Hello, world!
}

func ExampleImport() {
	out, err := exec.Command("date").Output()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("The date is %s\n", out)
}

func ExampleKeyValue() {
	v := struct {
		a string
		b int
	}{
		a: "A",
		b: 1,
	}
	fmt.Print(v)
	// Output: a: "A", b: 1
}

func ExampleKeyValueImport() {
	f := flag.Flag{
		Name: "play",
	}
	fmt.Print(f)
	// Output: Name: "play"
}

var keyValueTopDecl = struct {
	a string
	b int
}{
	a: "B",
	b: 2,
}

func ExampleKeyValueTopDecl() {
	fmt.Print(keyValueTopDecl)
	// Output: a: "B", b: 2
}

// Person 表示a person by name and age.
type Person struct {
	Name string
	Age  int
}

// String 返回a string representation of the Person.
func (p Person) String() string {
	return fmt.Sprintf("%s: %d", p.Name, p.Age)
}

// ByAge 实现sort.Interface for []Person based on
// the Age field.
type ByAge []Person

// Len 返回the number of elements in ByAge.
func (a ByAge) Len() int { return len(a) }

// Swap swaps the elements in ByAge.
func (a ByAge) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByAge) Less(i, j int) bool { return a[i].Age < a[j].Age }

// people 是 array of Person
var people = []Person{
	{"Bob", 31},
	{"John", 42},
	{"Michael", 17},
	{"Jenny", 26},
}

func ExampleSort() {
	fmt.Println(people)
	sort.Sort(ByAge(people))
	fmt.Println(people)
	// Output:
	// [Bob: 31 John: 42 Michael: 17 Jenny: 26]
	// [Michael: 17 Jenny: 26 Bob: 31 John: 42]
}
