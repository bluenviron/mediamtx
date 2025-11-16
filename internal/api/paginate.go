package api

import (
	"fmt"
	"reflect"
	"strconv"
)

func paginate2(itemsPtr any, itemsPerPage int, page int) int {
	ritems := reflect.ValueOf(itemsPtr).Elem()

	itemsLen := ritems.Len()
	if itemsLen == 0 {
		return 0
	}

	pageCount := (itemsLen / itemsPerPage)
	if (itemsLen % itemsPerPage) != 0 {
		pageCount++
	}

	minVal := min(page*itemsPerPage, itemsLen)

	maxVal := min((page+1)*itemsPerPage, itemsLen)

	ritems.Set(ritems.Slice(minVal, maxVal))

	return pageCount
}

func paginate(itemsPtr any, itemsPerPageStr string, pageStr string) (int, error) {
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
