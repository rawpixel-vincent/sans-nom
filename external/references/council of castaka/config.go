package council

type RoleName string

var (
	// Council des Meta Barons
	RoleHonorata                   RoleName = "honorata"                       // the first Metabaron's wife and the dynasty's true architect.
	RoleAghora                     RoleName = "aghora"                         // the Père-Mère
	RoleOda                        RoleName = "oda"                            // the Capricious
	RoleAghnarVonSalza             RoleName = "aghnar-von-salza"               // the first Metabaron
	RoleDonaVicentaGabrielaDeRokha RoleName = "dona-vicenta-gabriela-de-rokha" // the only person in the universe who ever made a Metabaron question his own existence without drawing a weapon.
	RoleTeteDacier                 RoleName = "tete-dacier"                    // the fourth Metabaron
	RoleSansNom                    RoleName = "sans-nom"                       // the last and most powerful Metabaron.
	RoleOthonVonSalza              RoleName = "othon-von-salza"                // the first Metabaron
	// Invocation of the Council des Meta Barons
	RoleCouncilMetaBarons RoleName = "council-meta-barons"
)

func MetaBaronDisplayTitle(role RoleName) string {

	switch role {
	case RoleHonorata:
		return "Honorata — La Trisaïeule"
	case RoleAghora:
		return "Aghora — Le Père-Mère"
	case RoleOda:
		return "Oda — La Bisaïeule"
	case RoleAghnarVonSalza:
		return "Aghnar von Salza — Le Bisaïeul"
	case RoleDonaVicentaGabrielaDeRokha:
		return "Doña Vicenta Gabriela de Rokha — L'Aïeule"
	case RoleTeteDacier:
		return "Tête d'Acier (Steelhead) — L'Aïeul"
	case RoleSansNom:
		return "Sans-Nom — Le Dernier Méta-Baron"
	case RoleOthonVonSalza:
		return "Othon von Salza — Le Trisaïeul"
	}
	return ""
}
