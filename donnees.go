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

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	SpotifyclientID     = "938ba4f5f5b44d0a9c33bbe4e6ff349b"
	SpotifyclientSecret = "1f99f3d3b49740939cb2c4fb7eef78a1"
)

// var mongoURI = "mongodb+srv://celine21106:NVwe27nqvJ2TCt1Y@cluster0.chcv8d1.mongodb.net/?retryWrites=true&w=majority&appName=Cluster0"

var mongoURI = os.Getenv("MONGO_URI")

// Connexion à la base de données MongoDB
func connectToMongo() (*mongo.Client, error) {
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, err
	}
	return client, nil
}

type Artist struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Popularity int      `json:"popularity"`
	Genre      []string `json:"genre"`
}

type Track struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Popularity int      `json:"popularity"`
	Artists    []string `json:"artists"`
	Country    string   `json:"country"`
}

type PlaylistCountry struct {
	PlaylistID string
	Country    string
}

type CountryTracks struct {
	Country string  `json:"country"`
	Tracks  []Track `json:"tracks"`
}

// liste des pays dont on récuperera leur playlist TOP50 officiel de Spotify
func createTOP50Playlists() []PlaylistCountry {
	list := []PlaylistCountry{
		{"37i9dQZEVXbLRQDuF5jeBp", "USA"},
		{"37i9dQZEVXbIPWwFssbupI", "France"},
		{"37i9dQZEVXbLnolsZ8PSNw", "UK"},
		{"37i9dQZEVXbIVYVBNw9D5K", "Turkey"},
		{"37i9dQZEVXbNFJfN1Vw8d9", "Spain"},
		{"37i9dQZEVXbNxXF4SkHj9F", "South Korea"},
		{"37i9dQZEVXbJiZcmkrIHGU", "Germany"},
	}
	return list
}

// sauvegarde les artistes dans la collection artists de MongoDB
func saveArtists(artistDetails []Artist, artistsCollection *mongo.Collection) error {
	for _, artist := range artistDetails {
		_, err := artistsCollection.UpdateOne(
			context.TODO(),
			bson.M{"id": artist.ID},
			bson.M{"$set": artist},
			options.Update().SetUpsert(true),
		)
		if err != nil {
			return fmt.Errorf("erreur lors de la sauvegarde de l'artiste %s dans MongoDB: %w", artist.Name, err)
		}
	}
	return nil
}

// extrait les noms et la popularité des artistes
func extractArtistNameAndPopularity(artistsData []interface{}) ([]string, []Artist) {
	var artistNames []string
	var artistDetails []Artist
	for _, artistData := range artistsData {
		artistMap, ok := artistData.(map[string]interface{})
		if !ok {
			continue
		}
		artistID, _ := artistMap["id"].(string)
		artistName, _ := artistMap["name"].(string)
		artistPopularity, _ := artistMap["popularity"].(int)

		artistNames = append(artistNames, artistName)
		artistDetails = append(artistDetails, Artist{
			ID:         artistID,
			Name:       artistName,
			Popularity: artistPopularity,
		})
	}
	return artistNames, artistDetails
}

// récupère les pistes d'une playlist Spotify, extrait les détails des pistes
// et des artistes, et sauvegarde ces informations dans MongoDB
func saveTracksFromPlaylist(playlistID string, country string) ([]Track, error) {
	var tracks []Track //pour le retour

	token, err := getAccessToken()
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la récupération du token d'accès: %w", err)
	}

	tracksURL := fmt.Sprintf("https://api.spotify.com/v1/playlists/%s/tracks", playlistID)

	req, err := http.NewRequest("GET", tracksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la création de la requête: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+token)

	//envoi de la requete pour récupérer la liste de tracks dans la playlist
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de l'envoi de la requête: %w", err)
	}
	defer resp.Body.Close()

	//lecture de la réponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la lecture de la réponse: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, fmt.Errorf("erreur lors du décodage de la réponse: %w", err)
	}

	client, err := connectToMongo()
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la connexion à MongoDB: %w", err)
	}
	defer client.Disconnect(context.TODO())

	artistsCollection := client.Database("spotifyData").Collection("artists")

	items, ok := result["items"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("impossible de convertir la valeur de 'items' en []interface{}")
	}

	//parcourt la liste de tracks et extrait les différentes données pour les sauvegarder dans la collection correspondante
	for _, item := range items {
		trackMap, ok := item.(map[string]interface{})["track"].(map[string]interface{})
		if !ok {
			continue
		}
		artistNames, artistDetails := extractArtistNameAndPopularity(trackMap["artists"].([]interface{}))

		track := Track{
			ID:         trackMap["id"].(string),
			Name:       trackMap["name"].(string),
			Popularity: int(trackMap["popularity"].(float64)),
			Artists:    artistNames,
			Country:    country,
		}

		tracks = append(tracks, track)
		if err := saveArtists(artistDetails, artistsCollection); err != nil {
			return tracks, fmt.Errorf("erreur lors d'enregistrement d'artist en saveArtists")
		}
	}

	return tracks, nil
}

// Fonction principale pour créer et remplir la collection 'top50'
func saveTop50Playlists(playlists []PlaylistCountry) error {
	client, err := connectToMongo()
	if err != nil {
		return fmt.Errorf("erreur lors de la connexion à MongoDB: %w", err)
	}
	defer client.Disconnect(context.TODO())

	top50Collection := client.Database("spotifyData").Collection("top50")
	//vide la collection 'top50' avant l'insertion des nouveaux documents
	_, err = top50Collection.DeleteMany(context.Background(), bson.M{})
	if err != nil {
		return fmt.Errorf("erreur lors du vidage de la collection top50: %w", err)
	}

	//récupère les tracks pour chaque playlist et les sauvegarder
	for _, pc := range playlists {
		tracks, err := saveTracksFromPlaylist(pc.PlaylistID, pc.Country)
		if err != nil {
			log.Printf("Erreur lors de la récupération des tracks pour le pays %s: %v", pc.Country, err)
			continue
		}

		//crée un document pour le pays avec sa liste de tracks
		countryTracks := CountryTracks{
			Country: pc.Country,
			Tracks:  tracks,
		}

		// insére le document dans la collection 'top50'
		_, err = top50Collection.InsertOne(context.Background(), countryTracks)
		if err != nil {
			log.Printf("Erreur lors de l'insertion des tracks pour le pays %s dans la collection top50: %v", pc.Country, err)
		}
	}

	return nil
}

// Met à jour la popularité et genre des artistes dans la collection artists
func updateArtistsPopularityAndGenre() error {

	client, err := connectToMongo()
	if err != nil {
		return fmt.Errorf("erreur lors de la connexion à MongoDB: %w", err)
	}
	defer client.Disconnect(context.TODO())

	artistsCollection := client.Database("spotifyData").Collection("artists")

	cursor, err := artistsCollection.Find(context.TODO(), bson.M{})
	if err != nil {
		return fmt.Errorf("erreur lors de la recherche des artistes: %w", err)
	}
	defer cursor.Close(context.TODO())

	var artists []Artist
	if err = cursor.All(context.TODO(), &artists); err != nil {
		return fmt.Errorf("erreur lors de la lecture des documents de la collection: %w", err)
	}

	token, err := getAccessToken()
	if err != nil {
		return fmt.Errorf("erreur lors de la récupération du token d'accès: %w", err)
	}

	for _, artist := range artists {
		//fait une requête GET à l'API Spotify pour cet artiste en utilisant artist.ID
		artistURL := fmt.Sprintf("https://api.spotify.com/v1/artists/%s", artist.ID)

		req, err := http.NewRequest("GET", artistURL, nil)
		if err != nil {
			log.Printf("erreur lors de la création de la requête pour l'artiste %s: %v", artist.ID, err)
			continue
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("erreur lors de l'envoi de la requête pour l'artiste %s: %v", artist.ID, err)
			continue
		}
		defer resp.Body.Close()

		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("erreur lors de la lecture de la réponse pour l'artiste %s: %v", artist.ID, err)
			continue
		}

		var spotifyArtist struct {
			Popularity int      `json:"popularity"`
			Genres     []string `json:"genres"`
		}
		if err := json.Unmarshal(responseBody, &spotifyArtist); err != nil {
			log.Printf("Erreur lors du décodage de la réponse pour l'artiste %s: %v", artist.ID, err)
			continue
		}

		// met à jour la popularité et les genres de l'artiste dans MongoDB
		update := bson.M{
			"$set": bson.M{
				"popularity": spotifyArtist.Popularity,
				"genre":      spotifyArtist.Genres,
			},
		}
		filter := bson.M{"id": artist.ID}

		if _, err := artistsCollection.UpdateOne(context.TODO(), filter, update); err != nil {
			log.Printf("Erreur lors de la mise à jour de l'artiste %s: %v", artist.ID, err)
		}
	}
	fmt.Println("updateArtistsPopularityAndGenre is done!")

	return nil
}

func getAccessToken() (string, error) {
	// URL pour obtenir le token d'accès
	tokenURL := "https://accounts.spotify.com/api/token"

	requestBody := url.Values{}
	requestBody.Set("grant_type", "client_credentials")
	requestBody.Set("client_id", SpotifyclientID)
	requestBody.Set("client_secret", SpotifyclientSecret)

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(requestBody.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// envoye la requête
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Lit la réponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Extrait le token d'accès
	var result map[string]interface{}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return "", err
	}
	token := result["access_token"].(string)

	return token, nil
}
