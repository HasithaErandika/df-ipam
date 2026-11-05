package main

const provider = "demo"

// Sample people can't sign in. They exist so that second-person rules can be met.
var cast = []struct{ Name, Display, Role string }{
	{"admin", "Sample Administrator", "Super Administrator"}, {"approver", "Sample Approver", "IPAM Administrator"},
	{"approver2", "Sample Second Approver", "IPAM Administrator"}, {"engineer", "Sample Network Engineer", "Network Engineer"},
	{"engineer2", "Sample Site Engineer", "Network Engineer"}, {"requester", "Sample Server Engineer", "Infrastructure Engineer"},
	{"requester2", "Sample Application Owner", "Infrastructure Engineer"}, {"auditor", "Sample Auditor", "Security Auditor"},
}

func emailOf(name string) string { return "sample." + name + "@example.com" }

func (s *sample) alreadySeeded() bool {
	var found bool
	if err := s.pool.QueryRow(s.ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE auth_provider = $1)`, provider).Scan(&found); err != nil {
		panic(err)
	}
	return found
}

func (s *sample) addPeople() {
	for _, p := range cast {
		var id string
		err := s.pool.QueryRow(s.ctx, `INSERT INTO users (email, display_name, auth_provider, auth_subject) VALUES ($1, $2, $3, $4) RETURNING id::text`,
			emailOf(p.Name), p.Display, provider, p.Name).Scan(&id)
		if err != nil {
			panic(err)
		}
		s.people[p.Name] = id
		if _, err := s.pool.Exec(s.ctx, `INSERT INTO user_roles (user_id, role_id) SELECT $1::uuid, id FROM roles WHERE name = $2`, id, p.Role); err != nil {
			panic(err)
		}
	}
}
