package addressdep

type Item struct{ Value int }
type Owner struct{ item Item }

func Embedded(owner *Owner) *Item { return &owner.item }
func Use(item *Item) int          { return item.Value }
