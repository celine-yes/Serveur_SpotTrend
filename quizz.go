package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type QuestionTrend struct {
	Question string   `json:"question"`
	Choices  []string `json:"choices"`
	Answer   string   `json:"answer"`
}

type QuizResult struct {
	UserID string
	Score  int
}

const (
	TopArtistsQuestionType = iota
	GenreQuestionType
	RegionalTrendsQuestionType
)

// handler pour générer une question de quiz
func generateQuizQuestionHandler(w http.ResponseWriter, r *http.Request) {
	// Autoriser les CORS, ajustez selon vos besoins
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Gérer la pré-vérification CORS
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	var question QuestionTrend
	var err error
	// Connexion à MongoDB
	client, err := connectToMongo()
	if err != nil {
		http.Error(w, "Erreur lors de la connexion à MongoDB: "+err.Error(), http.StatusInternalServerError)
		log.Println(err)
		return
	}
	defer client.Disconnect(context.TODO())

	db := client.Database("spotifyData")

	validQuestion := false
	for attempts := 0; attempts < 3 && !validQuestion; attempts++ {
		questionType := rand.Intn(3)

		// Générer la question en fonction du type
		switch questionType {
		case TopArtistsQuestionType:
			question, err = generateTopArtistsQuestion(db)
		case GenreQuestionType:
			question, err = generateGenreQuestion(db)
		case RegionalTrendsQuestionType:
			question, err = generateRegionalTrendsQuestion(db)
		}

		if err == nil && len(question.Choices) > 0 {
			validQuestion = true
		}
	}

	if !validQuestion {
		http.Error(w, "Impossible de générer une question valide après plusieurs tentatives", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(question)
}

func generateTopArtistsQuestion(db *mongo.Database) (QuestionTrend, error) {
	//log.Printf("dans generateTopArtistsQuestion")
	artistsCollection := db.Collection("artists")

	// Sélectionner 4 artistes aléatoires
	pipeline := mongo.Pipeline{
		{{Key: "$sample", Value: bson.D{{Key: "size", Value: 4}}}},
	}
	cursor, err := artistsCollection.Aggregate(context.Background(), pipeline)
	if err != nil {
		log.Printf("Erreur lors de l'agrégation des artistes: %v", err)
		return QuestionTrend{}, fmt.Errorf("erreur lors de l'agrégation des artistes: %w", err)
	}
	defer cursor.Close(context.TODO())

	var artists []Artist
	if err = cursor.All(context.TODO(), &artists); err != nil {
		log.Printf("Erreur lors de la récupération des artistes: %v", err)
		return QuestionTrend{}, fmt.Errorf("erreur lors de la récupération des artistes: %w", err)
	}

	question := QuestionTrend{
		Choices:  make([]string, 0, 4),
		Question: "Quel est l'artiste le plus streamé?",
	}

	if len(artists) == 0 {
		log.Println("Aucun artiste trouvé")
		return QuestionTrend{}, fmt.Errorf("aucun artiste trouvé")
	}

	var mostPopular Artist
	for _, artist := range artists {
		question.Choices = append(question.Choices, artist.Name)
		if mostPopular.Name == "" || artist.Popularity > mostPopular.Popularity {
			mostPopular = artist
		}
	}

	if mostPopular.Name != "" {
		question.Answer = mostPopular.Name
	} else {
		log.Println("Aucun artiste populaire trouvé ou données de popularité manquantes")
		return QuestionTrend{}, fmt.Errorf("aucun artiste populaire trouvé ou données de popularité manquantes")
	}

	return question, nil
}

func generateGenreQuestion(db *mongo.Database) (QuestionTrend, error) {

	genres := []string{"j-pop", "rock", "jazz", "blues", "classical", "rap",
		"r&b", "pop", "hip hop", "french hip hop", "k-pop"}

	// Choisissez aléatoirement un genre principal et un genre secondaire
	rand.Shuffle(len(genres), func(i, j int) { genres[i], genres[j] = genres[j], genres[i] })
	mainGenre := genres[0]
	secondaryGenre := genres[1] // Assurez-vous que c'est différent du premier

	// Construction de la requête pour sélectionner les artistes
	artistsCollection := db.Collection("artists")
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{{Key: "genre", Value: mainGenre}}}},
		bson.D{{Key: "$sample", Value: bson.D{{Key: "size", Value: 3}}}},
		bson.D{{Key: "$unionWith", Value: bson.D{
			{Key: "coll", Value: "artists"},
			{Key: "pipeline", Value: mongo.Pipeline{
				bson.D{{Key: "$match", Value: bson.D{{Key: "genre", Value: secondaryGenre}}}},
				bson.D{{Key: "$sample", Value: bson.D{{Key: "size", Value: 1}}}},
			}},
		}}},
	}

	cursor, err := artistsCollection.Aggregate(context.Background(), pipeline)
	if err != nil {
		return QuestionTrend{}, fmt.Errorf("erreur lors de l'agrégation des artistes: %w", err)
	}
	defer cursor.Close(context.TODO())

	var artists []Artist
	if err = cursor.All(context.TODO(), &artists); err != nil {
		return QuestionTrend{}, fmt.Errorf("erreur lors de la récupération des artistes: %w", err)
	}

	if len(artists) == 0 {
		return QuestionTrend{}, fmt.Errorf("aucun artiste trouvé pour générer une question de genre")
	}

	// Créer les choix de réponse
	question := QuestionTrend{
		Question: fmt.Sprintf("Quel est le genre musical le plus représenté parmi ces artistes : %s,  %s,  %s,  %s",
			artists[0].Name,
			artists[1].Name,
			artists[2].Name,
			artists[3].Name),
		Answer:  mainGenre,
		Choices: make([]string, 0, 4),
	}
	question.Choices = append(question.Choices, mainGenre)
	question.Choices = append(question.Choices, secondaryGenre)

	rand.Shuffle(len(genres), func(i, j int) { genres[i], genres[j] = genres[j], genres[i] })

	for _, genre := range genres {
		if genre != mainGenre && genre != secondaryGenre && len(question.Choices) < 4 {
			question.Choices = append(question.Choices, genre)
		}
	}

	// Mélanger les choix
	rand.Shuffle(len(question.Choices), func(i, j int) { question.Choices[i], question.Choices[j] = question.Choices[j], question.Choices[i] })

	return question, nil
}
func generateRegionalTrendsQuestion(db *mongo.Database) (QuestionTrend, error) {
	top50Collection := db.Collection("top50")
	// Sélection de 4 documents aléatoires de la collection 'top50'
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$sample", Value: bson.D{{Key: "size", Value: 4}}}},
	}

	cursor, err := top50Collection.Aggregate(context.Background(), pipeline)
	if err != nil {
		return QuestionTrend{}, fmt.Errorf("erreur lors de l'agrégation pour une tendance régionale: %w", err)
	}
	defer cursor.Close(context.TODO())

	var countryTracksList []CountryTracks
	if err = cursor.All(context.TODO(), &countryTracksList); err != nil {
		return QuestionTrend{}, fmt.Errorf("erreur lors de la récupération des données de tendance régionale: %w", err)
	}

	if len(countryTracksList) < 4 {
		return QuestionTrend{}, fmt.Errorf("pas assez de données de tendance régionale trouvées")
	}

	questionType := rand.Intn(2) // Génère aléatoirement 0 ou 1
	var question QuestionTrend
	question.Choices = make([]string, 0, 4)

	if questionType == 0 {
		selectedTrackIndex := rand.Intn(len(countryTracksList[0].Tracks))
		selectedTrack := countryTracksList[0].Tracks[selectedTrackIndex]

		question.Question = fmt.Sprintf("Dans quel pays la piste '%s' est-elle la plus populaire?", selectedTrack.Name)
		question.Answer = countryTracksList[0].Country

		for _, countryTracks := range countryTracksList[1:] {
			question.Choices = append(question.Choices, countryTracks.Country)
		}
		question.Choices = append(question.Choices, question.Answer)
	} else {
		selectedCountryTracks := countryTracksList[0]
		question.Question = fmt.Sprintf("Quelle est la piste la plus populaire en %s?", selectedCountryTracks.Country)
		mostPopular := selectedCountryTracks.Tracks[0] // La piste la plus populaire est à l'indice 0
		question.Answer = mostPopular.Name

		for i, track := range selectedCountryTracks.Tracks {
			if i > 0 && len(question.Choices) < 3 {
				question.Choices = append(question.Choices, track.Name)
			}
		}
		question.Choices = append(question.Choices, question.Answer)
	}

	rand.Shuffle(len(question.Choices), func(i, j int) { question.Choices[i], question.Choices[j] = question.Choices[j], question.Choices[i] })

	return question, nil
}

// handler pour finir un quiz et mettre à jour les infos de l'utilisateur
func finishQuizHandler(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	// Gérer la pré-vérification CORS
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Printf("Received token: %s", r.Header.Get("Authorization"))

	// Extraire le token de l'en-tête Authorization
	tokenHeader := r.Header.Get("Authorization")
	if tokenHeader == "" {
		http.Error(w, "Authorization header is required", http.StatusUnauthorized)
		return
	}

	splitToken := strings.Split(tokenHeader, "Bearer ")
	if len(splitToken) != 2 {
		http.Error(w, "Invalid Authorization token", http.StatusUnauthorized)
		return
	}

	tokenString := splitToken[1]
	claims := &jwt.StandardClaims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})

	// Vérification du token
	if err != nil || !token.Valid {
		http.Error(w, "Invalid Authorization token", http.StatusUnauthorized)
		return
	}

	log.Printf("UserID from token: '%s'", claims.Subject)

	userID := strings.TrimSpace(claims.Subject)

	if _, ok := token.Claims.(*jwt.StandardClaims); ok && token.Valid {
		log.Printf("UserID from token: '%s'", userID)

		//décode les données JSON de la fin du quiz
		var quizResult QuizResult
		err = json.NewDecoder(r.Body).Decode(&quizResult)
		if err != nil {
			http.Error(w, "Erreur lors de la lecture des données JSON", http.StatusBadRequest)
			return
		}

		client, err := connectToMongo()
		if err != nil {
			http.Error(w, "Erreur lors de la connexion à MongoDB", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		defer client.Disconnect(context.Background())

		//mise à jour de l'utilisateur
		collection := client.Database("spotTrendQuizzer").Collection("users")
		filter := bson.M{"userid": userID}
		log.Println("userID", filter)
		// mise à jour du score total, du nombre de parties, et ajouter le score à l'historique
		update := bson.D{
			{Key: "$inc", Value: bson.M{
				"scoretotal":  quizResult.Score,
				"nbdeparties": 1,
			}},
			{Key: "$push", Value: bson.M{
				"scorehistory": bson.M{
					"$each":  []interface{}{fmt.Sprintf("%d", quizResult.Score)},
					"$slice": -5,
				},
			}},
		}

		_, err = collection.UpdateOne(context.Background(), filter, update)
		if err != nil {
			http.Error(w, "Erreur lors de la mise à jour du score de l'utilisateur", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		//mise à jour du score total dans la collection 'classement'
		classementCollection := client.Database("spotTrendQuizzer").Collection("classement")
		classementUpdate := bson.M{
			"$inc": bson.M{"scoreTotal": quizResult.Score},
		}
		_, err = classementCollection.UpdateOne(context.Background(), bson.M{"userId": userID}, classementUpdate)
		if err != nil {
			http.Error(w, "Erreur lors de la mise à jour du score total dans le classement", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		//calcul du nouveau classement de l'utilisateur après la mise à jour du score total
		userRanking, err := getRanking(client, userID)
		if err != nil {
			http.Error(w, "Erreur lors de la récupération du nouveau classement", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		//mettre à jour le classement de l'utilisateur dans la collection 'users'
		_, err = collection.UpdateOne(
			context.Background(),
			bson.M{"userid": userID},
			bson.M{"$set": bson.M{"userRanking": userRanking}},
		)
		if err != nil {
			http.Error(w, "Erreur lors de la mise à jour du classement de l'utilisateur", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Mise à jour réussie")
	}
}

func getQuizResultHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	// Gérer la pré-vérification CORS
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	tokenHeader := r.Header.Get("Authorization")
	splitToken := strings.Split(tokenHeader, "Bearer ")
	if len(splitToken) != 2 {
		http.Error(w, "Invalid Authorization token", http.StatusUnauthorized)
		return
	}
	tokenString := splitToken[1]

	// Parse et validation du token JWT
	claims := &jwt.StandardClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	if err != nil || !token.Valid {
		http.Error(w, "Invalid Authorization token", http.StatusUnauthorized)
		return
	}

	// Connexion à MongoDB
	client, err := connectToMongo()
	if err != nil {
		http.Error(w, "Erreur lors de la connexion à MongoDB", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	defer client.Disconnect(context.Background())

	// Récupération des informations de l'utilisateur
	//userID := claims.Subject
	usersCollection := client.Database("spotTrendQuizzer").Collection("users")
	//userFilter := bson.M{"userid": userID}
	//Utilise l'ID de l'utilisateur pour récupérer ses informations
	var user User
	log.Printf(claims.Subject)
	err = usersCollection.FindOne(context.Background(), bson.M{"userid": claims.Subject}).Decode(&user)
	if err != nil {
		http.Error(w, "Erreur lors de la récupération des informations de l'utilisateur", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	//Récupère le classement de l'utilisateur
	userRanking, err := getRanking(client, user.UserID)
	if err != nil {
		http.Error(w, "Erreur lors de la récupération du classement", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	//création de la réponse
	response := struct {
		ScoreTotal  int           `json:"scoreTotal"`
		UserRanking []UserRanking `json:"userRanking"`
	}{
		ScoreTotal:  user.ScoreTotal,
		UserRanking: userRanking,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
