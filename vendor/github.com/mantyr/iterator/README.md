# Golang Iterator for ordered map

[![Build Status](https://travis-ci.org/mantyr/iterator.svg)](https://travis-ci.org/mantyr/iterator) [![GoDoc](https://godoc.org/github.com/mantyr/iterator?status.png)](http://godoc.org/github.com/mantyr/iterator)

1. The order of the elements in the map.

2. Transparent use *interface{} as value.

## Installation

  $ go get github.com/mantyr/iterator

## Examples

```Go
package main

import (
    "github.com/mantyr/iterator"
    "fmt"
    "strconv"
)

type Obj struct {
    param1 int
    param2 string
}

func main() {
    items := iterator.New()

    items.Add("test1", "value1")
    items.Add("test2", "value2")
    items.Add("test2", "value2")
    items.Add("test3", "value3")
    items.Add("test4", "value4")

    fmt.Println("Ordered map:")
    for item := range items.Iter() {
        fmt.Printf("Position %q, key %q, value %q \r\n", strconv.Itoa(item.Index), item.Key, item.Value)
    }

    fmt.Println("Default map:")
    for key, value := range items.Items {
        fmt.Printf("Key %q, value %q \r\n", key, value)
    }

    // for *obj
    obj := new(Obj)
    obj.param1 = 10
    obj.param2 = "test1"

    items = iterator.New()
    items.Add("test5", obj)

    fmt.Println("Ordered map *:")
    for item := range items.Iter() {
        fmt.Printf("Position %q, key %q, value param2 %q \r\n", strconv.Itoa(item.Index), item.Key, item.Value.(*Obj).param2)
    }

    fmt.Println("Default map *:")
    for key, value := range items.Items {
        fmt.Printf("Key %q, value param2 %q \r\n", key, value.(*Obj).param2)
    }
}
/*  out
Ordered map:
Position "0", key "test1", value "value1" 
Position "1", key "test2", value "value2" 
Position "2", key "test3", value "value3" 
Position "3", key "test4", value "value4" 
Default map:
Key "test2", value "value2" 
Key "test3", value "value3" 
Key "test4", value "value4" 
Key "test1", value "value1" 
Ordered map *:
Position "0", key "test5", value param2 "test1" 
Default map *:
Key "test5", value param2 "test1"
*/
```

## Notice

Not use channel if you use "back" in loop. See also [#1](/../../issues/1)

```Go
package main

import (
    "github.com/mantyr/iterator"
)

func main() {
    items := iterator.New()
    for i := 0; i < 100; i++ {
        items.Add(i, i)
    }

    i := 0
    for _, key := range items.Keys {
        item, ok := items.Get(key)
        if !ok {
            continue
        }
        if item == 10 {
            break
        }
        i++
    }
}
```

## Example for html/template and helper function for objects

```Go
package main

import (
    "log"
    "net/http"
    "github.com/gorilla/context"
    "github.com/mantyr/iterator"
    "html/template"
)

func main() {
    http.HandleFunc("/test", handler_iterator)

    if err := http.ListenAndServe(":8080", context.ClearHandler(http.DefaultServeMux)); err != nil {
        log.Fatal("ListenAndServe:", err)
    }
}

type Obj struct {
    Id int
    Search string
}

type ObjMap struct {
    Values map[string]string
}
func (o *ObjMap) Get(key string) string {
    val, ok := o.Values[key]
    if ok {
        return val
    }
    return ""
}

func handler_iterator(w http.ResponseWriter, r *http.Request) {
    var tmpl = template.Must(template.ParseFiles("tmpl/index.html"))

    items_st := iterator.New()

    items_st.Add("test1", "value1")
    items_st.Add("test2", "value2")
    items_st.Add("test2", "value2")
    items_st.Add("test3", "value3")
    items_st.Add("test4", "value4")


    items := iterator.New()
    obj1 := Obj{1, "test1"}
    obj2 := Obj{2, "test2"}
    obj3 := Obj{3, "test3"}

    items.Add("test1", obj1)
    items.Add("test2", obj2)
    items.Add("test3", obj3)

    items_ob := iterator.New()
    obj_m1 := new(ObjMap)
    obj_m2 := new(ObjMap)
    obj_m3 := new(ObjMap)

    obj_m1.Values = make(map[string]string)
    obj_m1.Values["id"] = "1"
    obj_m1.Values["value"] = "test1"

    obj_m2.Values = make(map[string]string)
    obj_m2.Values["id"] = "2"
    obj_m2.Values["value"] = "test2"

    obj_m3.Values = make(map[string]string)
    obj_m3.Values["id"] = "3"
    obj_m3.Values["value"] = "test3"


    items_ob.Add("test1", obj_m1)
    items_ob.Add("test2", obj_m2)
    items_ob.Add("test3", obj_m3)

    data := struct {
        ItemsString <-chan iterator.Item
        Items       <-chan iterator.Item
        ItemsObj    <-chan iterator.Item
    }{items_st.Iter(), items.Iter(), items_ob.Iter()}

    tmpl.Execute(w, data)
}
```

    $ cat tmpl/index.html

```Html
<div class="content">
    {{range $item := .ItemsString}}
        <div class="item finish" data-id="{{$item.Key}}">{{$item.Value}}</div>
    {{end}}
</div>
<div class="content">
    {{range $item := .Items}}
        <div class="item finish" data-id="{{$item.Value.Id}}">{{$item.Value.Search}}</div>
    {{end}}
</div>
<div class="content">
    {{range $item := .ItemsObj}}
        <div class="item finish" data-id="{{$item.Value.Get "id"}}">{{$item.Value.Get "value"}}</div>
    {{end}}
</div>
```

    http://localhost:8080/test

```Html
<div class="content">
        <div class="item finish" data-id="test1">value1</div>
        <div class="item finish" data-id="test2">value2</div>
        <div class="item finish" data-id="test3">value3</div>
        <div class="item finish" data-id="test4">value4</div>
</div>
<div class="content">
        <div class="item finish" data-id="1">test1</div>
        <div class="item finish" data-id="2">test2</div>
        <div class="item finish" data-id="3">test3</div>
</div>
<div class="content">
        <div class="item finish" data-id="1">test1</div>
        <div class="item finish" data-id="2">test2</div>
        <div class="item finish" data-id="3">test3</div>
</div>
```

## More Examples

    $ cat iterator_test.go

## Author

[Oleg Shevelev][mantyr]

[mantyr]: https://github.com/mantyr
