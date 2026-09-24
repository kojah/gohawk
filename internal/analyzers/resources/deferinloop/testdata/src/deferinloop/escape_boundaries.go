package deferinloop

import "os"

// Sending the owner through a select makes its contents available to another
// participant. The graph's unconditional send effects alone do not capture
// this alternative. The private-container diagnostic counterpart remains in
// accumulatedNestedResources.
func publishedContainerAlternative(names []string, output chan *resourceOwner) error {
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		item := &resourceOwner{resource: file}
		select {
		case output <- item:
		default:
		}
		defer item.resource.Close()
	}
	return nil
}
