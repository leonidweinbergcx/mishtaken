package cmd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

type Raw struct {
	Apartments          float64
	ApartmentsForLocals float64
	Subscribers         float64
	SqmPrice            float64
	City                string
	LotteryID           string
}

func Execute() {
	req, err := http.NewRequest(http.MethodGet, `https://www.dira.moch.gov.il/api/Invoker?method=Projects&param=%3FfirstApplicantIdentityNumber%3D%26secondApplicantIdentityNumber%3D%26ProjectStatus%3D4%26Entitlement%3D1%26PageNumber%3D1%26PageSize%3D200%26`, nil)

	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	resp, err := (&http.Client{}).Do(req)

	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	log.Info().Msg("fetching metadata")
	log.Info().Msg(resp.Status)

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	var v map[string]interface{}
	err = json.Unmarshal(body, &v)

	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	var raws []Raw

	for _, input := range v["ProjectItems"].([]interface{}) {
		inputv := input.(map[string]interface{})

		raw := &Raw{}
		getProjectData(raw, inputv["LotteryNumber"].(string))

		raw.City = inputv["CityDescription"].(string)
		raw.SqmPrice = inputv["PricePerUnit"].(float64)
		raw.LotteryID = inputv["LotteryNumber"].(string)

		lotteryStageSummary := inputv["LotteryStageSummery"].(map[string]interface{})
		raw.Subscribers = lotteryStageSummary["TotalSubscribers"].(float64) - raw.ApartmentsForLocals

		raws = append(raws, *raw)
	}

	projects := winChancePerProject(raws)
	cities := winChancePerCity(raws)

	writeOut("./projects-"+time.Now().String()+".json", projects)
	writeOut("./cities-"+time.Now().String()+".json", cities)
}

func getProjectData(raw *Raw, lotteryNumber string) {
	reqUrlTemplate := `https://www.dira.moch.gov.il/api/Invoker?method=LotteryResult&param=%3FlotteryNumber%3D{lotteryNumber}%26firstApplicantIdentityNumber%3D%26secondApplicantIdentityNumber%3D%26LoginId%3D%26`
	reqUrl := strings.Replace(reqUrlTemplate, "{lotteryNumber}", lotteryNumber, 1)

	req, err := http.NewRequest(http.MethodGet, reqUrl, nil)

	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	resp, err := (&http.Client{}).Do(req)

	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	log.Info().Msg("fetching data for lottery id " + lotteryNumber)
	log.Info().Msg(resp.Status)

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	var v map[string]interface{}
	err = json.Unmarshal(body, &v)

	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	myLotteryResult := v["MyLotteryResult"].(map[string]interface{})

	raw.ApartmentsForLocals = myLotteryResult["LocalHousing"].(float64)
	raw.Apartments = myLotteryResult["ApartmentsCount"].(float64) - raw.ApartmentsForLocals
}

func winChancePerProject(raws []Raw) interface{} {
	var projectsArr []interface{}

	for _, raw := range raws {
		projectObj := map[string]interface{}{
			"City":        raw.City,
			"LotteryID":   raw.LotteryID,
			"Sqm price":   raw.SqmPrice,
			"Subscribers": raw.Subscribers,
			"Apartments":  raw.Apartments,
			"Win chance":  (raw.Apartments / raw.Subscribers) * 100,
		}

		projectsArr = append(projectsArr, projectObj)
	}

	sort.SliceStable(projectsArr, func(i, j int) bool {
		first := projectsArr[i].(map[string]interface{})
		second := projectsArr[j].(map[string]interface{})

		return first["Win chance"].(float64) > second["Win chance"].(float64)
	})

	for _, project := range projectsArr {
		projectv := project.(map[string]interface{})

		projectv["Win chance"] = fmt.Sprintf("%.2f", projectv["Win chance"])
		projectv["Sqm price"] = fmt.Sprintf("%.0f", projectv["Sqm price"])
	}

	return projectsArr
}

func winChancePerCity(raws []Raw) interface{} {
	rawmap := make(map[string][]Raw)

	for _, raw := range raws {
		if rawArr, has := rawmap[raw.City]; has {
			rawArr = append(rawArr, raw)
			rawmap[raw.City] = rawArr
		} else {
			rawmap[raw.City] = []Raw{raw}
		}
	}

	var cityArr []interface{}

	for city, raws := range rawmap {
		cityobj := map[string]interface{}{
			"City":           city,
			"Total projects": len(raws),
		}

		// calc sqm avg
		var avgSqm float64
		avgSqm = 0

		for _, raw := range raws {
			avgSqm += float64(raw.SqmPrice)
		}

		cityobj["Avg sqm price"] = avgSqm / float64(len(raws))

		// calc avg win chance
		var avgChance float64
		avgChance = 0

		for _, raw := range raws {
			avgChance += (raw.Apartments / raw.Subscribers)
		}

		cityobj["Avg chance to win a single project"] = (avgChance / float64(len(raws))) * 100

		// calc avg oursiders
		var avgResidents float64
		avgResidents = 0

		for _, raw := range raws {
			avgResidents += raw.Subscribers
		}

		avgResidents = avgResidents / float64(len(raws))
		cityobj["Avg city subscribers"] = avgResidents

		// calc total apartments
		var totalApartments float64
		totalApartments = 0

		for _, raw := range raws {
			totalApartments += raw.Apartments
		}

		cityobj["Apartments in the city"] = totalApartments
		cityobj["Chance to win the city"] = (totalApartments / avgResidents) * 100

		cityArr = append(cityArr, cityobj)
	}

	sort.SliceStable(cityArr, func(i, j int) bool {
		first := cityArr[i].(map[string]interface{})
		second := cityArr[j].(map[string]interface{})

		return first["Chance to win the city"].(float64) > second["Chance to win the city"].(float64)
	})

	for _, city := range cityArr {
		cityv := city.(map[string]interface{})

		cityv["Avg sqm price"] = fmt.Sprintf("%.0f", cityv["Avg sqm price"])
		cityv["Avg chance to win a single project"] = fmt.Sprintf("%.2f", cityv["Avg chance to win a single project"])
		cityv["Avg city subscribers"] = fmt.Sprintf("%.0f", cityv["Avg city subscribers"])
		cityv["Apartments in the city"] = fmt.Sprintf("%.0f", cityv["Apartments in the city"])
		cityv["Chance to win the city"] = fmt.Sprintf("%.2f", cityv["Chance to win the city"])
	}

	return cityArr
}

func writeOut(filename string, data interface{}) {
	bytes, err := json.MarshalIndent(data, "", "  ")

	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	if err := os.WriteFile(filename, bytes, 0600); err != nil {
		log.Fatal().Err(err).Msg("")
	}
}
