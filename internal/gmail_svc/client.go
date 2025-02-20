package gmailsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/jhawk7/bill-parser/internal/common"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := os.Getenv("GMAIL_API_TOKEN")
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		common.LogError(fmt.Errorf("token file not read or doesn't exist; %v", err), false)
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		common.LogError(fmt.Errorf("unable to read authorization code: %v", err), true)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		common.LogError(fmt.Errorf("unable to retrieve token from web: %v", err), true)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		common.LogError(fmt.Errorf("unable to cache oauth token: %v", err), true)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func InitGSvc() *gmail.Service {
	ctx := context.Background()
	b, err := os.ReadFile(os.Getenv("EMAIL_CREDS_FILE"))
	if err != nil {
		common.LogError(fmt.Errorf("unable to read client secret file: %v", err), true)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, gmail.GmailModifyScope)
	if err != nil {
		common.LogError(fmt.Errorf("unable to parse client secret file to config: %v", err), true)
	}
	client := getClient(config)

	svc, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		common.LogError(fmt.Errorf("unable to retrieve Gmail client: %v", err), true)
	}

	return svc
}
