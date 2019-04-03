package iterator

type Items struct {
    Keys []interface{}
    Items map[interface{}]interface{}
}

type Item struct {
    Index int
    Key interface{}
    Value interface{}
}

func New() (items *Items){
    items = new(Items)
    items.Items = make(map[interface{}]interface{})
    return
}

func (i *Items) Iter() <-chan Item {
    ch := make(chan Item, 100)
    go func() {
        defer close(ch)
        for index, key := range i.Keys {
            val, ok := i.Items[key]
            if ok {
                ch <- Item{index, key, val}
            }
        }
    }()
    return ch
}

func (i *Items) Add(key interface{}, value interface{}) {
    _, ok := i.Items[key]
    i.Items[key] = value
    if !ok {
        i.Keys = append(i.Keys, key)
    }
}
func (i *Items) Get(key interface{}) (interface{}, bool)  {
    value, ok := i.Items[key]
    return value, ok
}
func (i *Items) Del(key interface{}) {
    delete(i.Items, key)
    for id, val := range i.Keys {
        if val == key {
            i.Keys = append(i.Keys[:id], i.Keys[id+1:]...)
            return
        }
    }
}