package api

import (
	"fmt"
	"reflect"
	"strconv"
)

func paginate2(itemsPtr interface{}, itemsPerPage int, page int) int {
	ritems := reflect.ValueOf(itemsPtr).Elem()

	itemsLen := ritems.Len()
	if itemsLen == 0 {
		return 0
	}

	pageCount := (itemsLen / itemsPerPage)
	if (itemsLen % itemsPerPage) != 0 {
		pageCount++
	}

	min := page * itemsPerPage
	if min > itemsLen {
		min = itemsLen
	}

	max := (page + 1) * itemsPerPage
	if max > itemsLen {
		max = itemsLen
	}

	ritems.Set(ritems.Slice(min, max))

	return pageCount
}

func paginate(itemsPtr interface{}, itemsPerPageStr string, pageStr string) (int, error) {
	itemsPerPage := 100

	if itemsPerPageStr != "" {
		tmp, err := strconv.ParseUint(itemsPerPageStr, 10, 31)
		if err != nil {
			return 0, err
		}
		itemsPerPage = int(tmp)

		if itemsPerPage == 0 {
			return 0, fmt.Errorf("invalid items per page")
		}
	}

	page := 0

	if pageStr != "" {
		tmp, err := strconv.ParseUint(pageStr, 10, 31)
		if err != nil {
			return 0, err
		}
		page = int(tmp)
	}

	return paginate2(itemsPtr, itemsPerPage, page), nil
}
