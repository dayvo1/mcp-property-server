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

	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type SearchPropertiesInput struct {
	City         string `json:"city" jsonschema:"the city to search in, e.g. Austin"`
	State        string `json:"state" jsonschema:"the 2-letter state abbreviation, e.g. TX"`
	MinYearBuilt int    `json:"minYearBuilt" jsonschema:"minimum year the property was built"`
	MinPrice     int    `json:"minPrice" jsonschema:"minimum listing price in dollars"`
}

type Property struct {
	Address     string `json:"address" jsonschema:"the property's full address"`
	Price       int    `json:"price" jsonschema:"the listed price in dollars"`
	YearBuilt   int    `json:"yearBuilt" jsonschema:"the year the property was built"`
	PhoneNumber string `json:"phoneNumber" jsonschema:"phone number attached to listing"`
}

type SearchPropertiesOutput struct {
	Properties []Property `json:"properties" jsonschema:"the list of matching properties found"`
}

func SearchPropertiesHandler(ctx context.Context, req *mcp.CallToolRequest, input SearchPropertiesInput) (
	*mcp.CallToolResult,
	SearchPropertiesOutput,
	error,
) {
	body, err := fetchListings(input.City, input.State, input.MinYearBuilt, input.MinPrice)
	if err != nil {
		return nil, SearchPropertiesOutput{}, err
	}

	var apiError struct {
		Status  int    `json:"status"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &apiError); err == nil && apiError.Error != "" {
		return nil, SearchPropertiesOutput{}, fmt.Errorf("RentCast error: %s", apiError.Message)
	}

	var listings []RentCastListing
	err = json.Unmarshal(body, &listings)
	if err != nil {
		return nil, SearchPropertiesOutput{}, err
	}

	var properties []Property

	for _, listing := range listings {
		properties = append(properties, Property{
			Address:     listing.FormattedAddress,
			Price:       listing.Price,
			YearBuilt:   listing.YearBuilt,
			PhoneNumber: listing.ListingAgent.Phone,
		})
	}
	return nil, SearchPropertiesOutput{Properties: properties}, nil
}

func fetchListings(city string, state string, minYearBuilt int, minPrice int) ([]byte, error) {
	baseURL := "https://api.rentcast.io/v1/listings/sale"

	params := url.Values{}
	params.Add("city", city)
	params.Add("state", state)
	params.Add("propertyType", "Single Family")
	params.Add("yearBuilt", fmt.Sprintf("%d:", minYearBuilt))
	params.Add("price", fmt.Sprintf("%d:", minPrice))

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

type ListingAgent struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Email string `json:"email"`
}

type RentCastListing struct {
	FormattedAddress string       `json:"formattedAddress"`
	Price            int          `json:"price"`
	YearBuilt        int          `json:"yearBuilt"`
	ListingAgent     ListingAgent `json:"listingAgent"`
}

func main() {
	godotenv.Load("/Users/david/Desktop/mcp-server/.env")

	server := mcp.NewServer(&mcp.Implementation{Name: "mcp-server", Version: "v1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "search-properties", Description: "fetch properties and their information"}, SearchPropertiesHandler)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}

}
