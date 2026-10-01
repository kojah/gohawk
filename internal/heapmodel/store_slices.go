package heapmodel

import (
	"go/types"
	"strconv"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Fixed views retain an offset into a local array; no backing-store alias set
// is inferred for arbitrary slice values. Bounds are checked before translating
// an element so different windows cannot silently name the same slot.
type storageArrayView struct {
	base              storageLocation
	offset, size, cap int64
}

func (storage *Storage) indexLocation(index *ssa.IndexAddr) (storageLocation, bool) {
	position, fixed := ssaflow.StorageInteger(index.Index, 0)
	view, known := storage.arrayView(index.X)
	if !fixed || !known || position < 0 || position >= view.size {
		return storageLocation{}, false
	}
	view.base.path += "/i" + strconv.FormatInt(view.offset+position, 10)
	return view.base, true
}

func (storage *Storage) arrayView(value ssa.Value) (storageArrayView, bool) {
	if value == nil || !storage.budget.Spend() {
		return storageArrayView{}, false
	}
	if sliced, ok := value.(*ssa.Slice); ok {
		base, known := storage.arrayView(sliced.X)
		low, lowOK := ssaflow.StorageInteger(sliced.Low, 0)
		high, highOK := ssaflow.StorageInteger(sliced.High, base.size)
		max, maxOK := ssaflow.StorageInteger(sliced.Max, base.cap)
		if !known || !lowOK || !highOK || !maxOK || low < 0 || low > high || high > max || max > base.cap {
			return storageArrayView{}, false
		}
		base.offset += low
		base.size, base.cap = high-low, max-low
		return base, true
	}
	pointer, ok := value.Type().Underlying().(*types.Pointer)
	if !ok {
		return storageArrayView{}, false
	}
	array, ok := pointer.Elem().Underlying().(*types.Array)
	if !ok {
		return storageArrayView{}, false
	}
	base, known := storage.location(value)
	return storageArrayView{base: base, size: array.Len(), cap: array.Len()}, known
}
