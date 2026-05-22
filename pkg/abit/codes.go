package abit

// Quota codes that may appear in Abiturient.Quotas. These are the
// canonical strings used by osvita.ua; consumers should compare against
// these constants rather than hard-coded literals.
const (
	QuotaKV1 = "КВ1" // territorial quota 1
	QuotaKV2 = "КВ2" // territorial quota 2
	QuotaKV3 = "КВ3" // territorial quota 3
	QuotaSB  = "СБ"  // interview-based admission (співбесіда)
)

// AllQuotas lists every quota code in display order. Useful for
// rendering filter UIs.
var AllQuotas = []string{QuotaKV1, QuotaKV2, QuotaKV3, QuotaSB}

// Coefficient codes that may appear in Abiturient.Coefficients.
const (
	CoefGK   = "ГК"  // галузевий коефіцієнт
	CoefSK   = "СК"  // сільський коефіцієнт
	CoefPCHK = "ПЧК" // південноукраїнський коефіцієнт
	CoefOL   = "ОЛ"  // олімпіадний коефіцієнт
	CoefKR   = "КР"  // коефіцієнт для контрактників
	CoefRK   = "РК"  // регіональний коефіцієнт
	CoefSB   = "СБ"  // співбесіда (also appears here in addition to Quotas)
)

// AllCoefficients lists every coefficient code in display order.
var AllCoefficients = []string{CoefGK, CoefSK, CoefPCHK, CoefOL, CoefKR, CoefRK, CoefSB}
