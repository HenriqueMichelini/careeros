package profilevalidation

type Experience struct {
	ID               string `json:"id"`
	Company          string `json:"company"`
	Title            string `json:"title"`
	StartDate        string `json:"startDate"`
	EndDate          string `json:"endDate"`
	Current          bool   `json:"current"`
	Location         string `json:"location"`
	Description      string `json:"description"`
	Responsibilities string `json:"responsibilities"`
	Achievements     string `json:"achievements"`
}

type Project struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Technologies string `json:"technologies"`
	URL          string `json:"url"`
	Highlights   string `json:"highlights"`
}

type Profile struct {
	CareerGoals      string       `json:"careerGoals"`
	Skills           string       `json:"skills"`
	Competencies     string       `json:"competencies"`
	Experience       []Experience `json:"experience"`
	Tools            string       `json:"tools"`
	Projects         []Project    `json:"projects"`
	EmploymentStatus string       `json:"employmentStatus"`
	CurrentSalary    string       `json:"currentSalary"`
	DesiredSalary    string       `json:"desiredSalary"`
	AdditionalInfo   string       `json:"additionalInfo"`
}

func Valid(profile Profile, maxTextBytes int) bool {
	for _, value := range []string{profile.CareerGoals, profile.Skills, profile.Competencies, profile.Tools, profile.EmploymentStatus, profile.CurrentSalary, profile.DesiredSalary, profile.AdditionalInfo} {
		if len(value) > maxTextBytes {
			return false
		}
	}
	if len(profile.Experience) > 40 || len(profile.Projects) > 40 {
		return false
	}
	for _, item := range profile.Experience {
		if item.ID == "" || len(item.ID) > 100 || !bounded(maxTextBytes, item.Company, item.Title, item.StartDate, item.EndDate, item.Location, item.Description, item.Responsibilities, item.Achievements) {
			return false
		}
	}
	for _, item := range profile.Projects {
		if item.ID == "" || len(item.ID) > 100 || !bounded(maxTextBytes, item.Name, item.Description, item.Technologies, item.URL, item.Highlights) {
			return false
		}
	}
	return true
}

func bounded(maxTextBytes int, values ...string) bool {
	for _, value := range values {
		if len(value) > maxTextBytes {
			return false
		}
	}
	return true
}
