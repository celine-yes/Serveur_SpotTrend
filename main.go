package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func main() {

	rand.Seed(time.Now().UnixNano())
	http.HandleFunc("/signup", signUpHandler)
	http.HandleFunc("/signin", signInHandler)
	http.HandleFunc("/userinfo", userInfoHandler)
	http.HandleFunc("/topPlayers", topPlayersHandler)
	http.HandleFunc("/generate-question", generateQuizQuestionHandler)
	http.HandleFunc("/finish-quizz", finishQuizHandler)
	http.HandleFunc("/get-result", getQuizResultHandler)

	createIndex()

	//listes des ids
	playlistTop50 := createTOP50Playlists()
	saveTop50Playlists(playlistTop50)

	//démarre le ticker pour exécuter les fonctions de récupération des données chaque heure
	go func() {
		ticker := time.NewTicker(24 * time.Hour) // actualise la bdd tous les 24h
		defer ticker.Stop()

		//execute apres selon le ticker
		for {
			select {
			case <-ticker.C:
				err := saveTop50Playlists(playlistTop50)
				if err != nil {
					log.Printf("Erreur lors de la sauvegarde des playlists Top 50: %v", err)
				}
				err = updateArtistsPopularityAndGenre()
				if err != nil {
					log.Printf("Erreur lors de l'update des artists: %v", err)
				}
			}
		}
	}()

	log.Println("Le serveur est démarré sur le port 8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Erreur lors du démarrage du serveur: %v", err)
	}
}

func createIndex() error {

	// Connexion à MongoDB
	client, err := connectToMongo()
	if err != nil {
		return fmt.Errorf("erreur lors de la connexion à MongoDB: %w", err)
	}
	defer client.Disconnect(context.TODO())

	collection := client.Database("spotTrendQuizzer").Collection("classement")
	indexModel := mongo.IndexModel{
		Keys: bson.M{"scoreTotal": -1}, // Index pour tri décroissant
	}
	_, err = collection.Indexes().CreateOne(context.Background(), indexModel)
	if err != nil {
		log.Fatal("Failed to create index for collection classement:", err)
	}
	log.Println("Index  for collection classement created successfully")
	return nil
}
