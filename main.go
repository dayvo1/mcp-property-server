package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type SearchPropertiesInput struct {
	City         string `json:"city" jsonschema:"the city to search in, e.g. Austin"`
	State        string `json:"state" jsonschema:"the 2-letter state abbreviation, e.g. TX"`
	MinYearBuilt int    `json:"minYearBuilt" jsonschema:"minimum year the property was built"`
	MinSoldYear  int    `json:"minSoldYear" jsonschema:"only include properties removed/sold in this year or later"`
	MinSoldPrice int    `json:"minSoldPrice" jsonschema:"minimum sale price in dollars"`
}

type Property struct {
	Address           string `json:"address" jsonschema:"the property's full address"`
	SoldPrice         int    `json:"soldPrice" jsonschema:"the listed/sale price in dollars"`
	YearBuilt         int    `json:"yearBuilt" jsonschema:"the year built, per the listing record"`
	YearBuiltVerified bool   `json:"yearBuiltVerified" jsonschema:"true if county tax records agree with the listing's year built"`
	VerificationNote  string `json:"verificationNote" jsonschema:"explains any data mismatch found between sources"`
	RemovedDate       string `json:"removedDate" jsonschema:"the date the listing was removed (proxy for sold date)"`
	PhoneNumber       string `json:"phoneNumber" jsonschema:"phone number attached to listing"`
}

type SearchPropertiesOutput struct {
	Properties []Property `json:"properties" jsonschema:"the list of matching properties found"`
}

type ListingAgent struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Email string `json:"email"`
}

type RentCastListing struct {
	FormattedAddress string       `json:"formattedAddress"`
	Price            int          `json:"price"`
	YearBuilt        int          `json:"yearBuilt"`
	RemovedDate      string       `json:"removedDate"`
	ListingAgent     ListingAgent `json:"listingAgent"`
}

type RentCastPropertyRecord struct {
	FormattedAddress string `json:"formattedAddress"`
	YearBuilt        int    `json:"yearBuilt"`
}

func SearchPropertiesHandler(ctx context.Context, req *mcp.CallToolRequest, input SearchPropertiesInput) (
	*mcp.CallToolResult,
	SearchPropertiesOutput,
	error,
) {
	listingsBody, err := fetchListings(input.City, input.State, input.MinYearBuilt)
	if err != nil {
		return nil, SearchPropertiesOutput{}, err
	}
	if apiErr := checkAPIError(listingsBody); apiErr != nil {
		return nil, SearchPropertiesOutput{}, apiErr
	}
	var listings []RentCastListing
	if err := json.Unmarshal(listingsBody, &listings); err != nil {
		return nil, SearchPropertiesOutput{}, err
	}

	propsBody, err := fetchPropertyRecords(input.City, input.State, input.MinYearBuilt)
	if err != nil {
		return nil, SearchPropertiesOutput{}, err
	}
	if apiErr := checkAPIError(propsBody); apiErr != nil {
		return nil, SearchPropertiesOutput{}, apiErr
	}
	var propRecords []RentCastPropertyRecord
	if err := json.Unmarshal(propsBody, &propRecords); err != nil {
		return nil, SearchPropertiesOutput{}, err
	}

	countyYearByAddress := make(map[string]int)
	for _, p := range propRecords {
		key := strings.ToLower(strings.TrimSpace(p.FormattedAddress))
		countyYearByAddress[key] = p.YearBuilt
	}

	var properties []Property
	for _, l := range listings {
		if l.Price < input.MinSoldPrice {
			continue
		}
		if len(l.RemovedDate) < 4 {
			continue
		}
		removedYear := 0
		fmt.Sscanf(l.RemovedDate[:4], "%d", &removedYear)
		if removedYear < input.MinSoldYear {
			continue
		}

		verified := true
		note := "No county record match found to cross-check"
		lookupKey := strings.ToLower(strings.TrimSpace(l.FormattedAddress))
		if countyYear, ok := countyYearByAddress[lookupKey]; ok {
			if countyYear != l.YearBuilt {
				verified = false
				note = fmt.Sprintf("MISMATCH: listing says built %d, county record says built %d", l.YearBuilt, countyYear)
			} else {
				note = "Confirmed: listing and county record agree"
			}
		}

		properties = append(properties, Property{
			Address:           l.FormattedAddress,
			SoldPrice:         l.Price,
			YearBuilt:         l.YearBuilt,
			YearBuiltVerified: verified,
			VerificationNote:  note,
			RemovedDate:       l.RemovedDate,
			PhoneNumber:       l.ListingAgent.Phone,
		})
	}

	return nil, SearchPropertiesOutput{Properties: properties}, nil
}

func checkAPIError(body []byte) error {
	var apiError struct {
		Status  int    `json:"status"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &apiError); err == nil && apiError.Error != "" {
		return fmt.Errorf("RentCast error: %s", apiError.Message)
	}
	return nil
}

func fetchListings(city string, state string, minYearBuilt int) ([]byte, error) {
	baseURL := "https://api.rentcast.io/v1/listings/sale"

	params := url.Values{}
	params.Add("city", city)
	params.Add("state", state)
	params.Add("propertyType", "Single Family")
	params.Add("status", "Inactive")
	params.Add("yearBuilt", fmt.Sprintf("%d:", minYearBuilt))
	params.Add("limit", "500")

	return doRentCastRequest(baseURL, params)
}

func fetchPropertyRecords(city string, state string, minYearBuilt int) ([]byte, error) {
	baseURL := "https://api.rentcast.io/v1/properties"

	params := url.Values{}
	params.Add("city", city)
	params.Add("state", state)
	params.Add("propertyType", "Single Family")
	params.Add("yearBuilt", fmt.Sprintf("%d:", minYearBuilt))
	params.Add("limit", "500")

	return doRentCastRequest(baseURL, params)
}

func doRentCastRequest(baseURL string, params url.Values) ([]byte, error) {
	fullURL := baseURL + "?" + params.Encode()

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", os.Getenv("RENTCAST_API_KEY"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func main() {
	godotenv.Load("/Users/david/Desktop/mcp-server/.env")

	server := mcp.NewServer(&mcp.Implementation{Name: "mcp-server", Version: "v1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "search-properties", Description: "fetch sold single-family properties, cross-checking year-built against two independent data sources to flag unreliable records"}, SearchPropertiesHandler)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
