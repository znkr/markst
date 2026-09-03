// Copyright 2026 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package lorem generates lorem ipsum placeholder text.
package lorem

import (
	"math/rand/v2"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const openingText = "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua."

var words = []string{"a", "ac", "accumsan", "ad", "adipiscing", "aenean", "aliquam", "aliquet", "amet", "ante", "aptent", "arcu", "at", "auctor", "augue", "bibendum", "blandit", "class", "commodo", "condimentum", "congue", "consectetur", "consequat", "conubia", "convallis", "cras", "cubilia", "curabitur", "curae", "cursus", "dapibus", "diam", "dictum", "dictumst", "dignissim", "dis", "dolor", "donec", "dui", "duis", "efficitur", "egestas", "eget", "eleifend", "elementum", "elit", "enim", "erat", "eros", "est", "et", "etiam", "eu", "euismod", "ex", "facilisi", "facilisis", "fames", "faucibus", "felis", "fermentum", "feugiat", "finibus", "fringilla", "fusce", "gravida", "habitant", "habitasse", "hac", "hendrerit", "himenaeos", "iaculis", "id", "imperdiet", "in", "inceptos", "integer", "interdum", "ipsum", "justo", "lacinia", "lacus", "laoreet", "lectus", "leo", "libero", "ligula", "litora", "lobortis", "lorem", "luctus", "maecenas", "magna", "magnis", "malesuada", "massa", "mattis", "mauris", "maximus", "metus", "mi", "molestie", "mollis", "montes", "morbi", "mus", "nam", "nascetur", "natoque", "nec", "neque", "netus", "nibh", "nisi", "nisl", "non", "nostra", "nulla", "nullam", "nunc", "odio", "orci", "ornare", "parturient", "pellentesque", "penatibus", "per", "pharetra", "phasellus", "placerat", "platea", "porta", "porttitor", "posuere", "potenti", "praesent", "pretium", "primis", "proin", "pulvinar", "purus", "quam", "quis", "quisque", "rhoncus", "ridiculus", "risus", "rutrum", "sagittis", "sapien", "scelerisque", "sed", "sem", "semper", "senectus", "sit", "sociosqu", "sodales", "sollicitudin", "suscipit", "suspendisse", "taciti", "tellus", "tempor", "tempus", "tincidunt", "torquent", "tortor", "tristique", "turpis", "ullamcorper", "ultrices", "ultricies", "urna", "ut", "varius", "vehicula", "vel", "velit", "venenatis", "vestibulum", "vitae", "vivamus", "viverra", "volutpat", "vulputate"}
var cumProp = []float64{0.013387792148854097, 0.02722940776038121, 0.03176764238711141, 0.03252401482489978, 0.03328038726268815, 0.0366840632327358, 0.045987444217532716, 0.0500718553815899, 0.06323273579910749, 0.07117464639588533, 0.0719310188336737, 0.07949474321155738, 0.09220180016640193, 0.09772331896225701, 0.1040768474396793, 0.10929581726041904, 0.11504424778761062, 0.11580062022539898, 0.12124650177747523, 0.12601164813554194, 0.13077679449360866, 0.1358444898267907, 0.13977762650329023, 0.1405339989410786, 0.14484532183647228, 0.14832463505029878, 0.14892973300052947, 0.1535436048710385, 0.1541487028212692, 0.15914076091067242, 0.16390590726873913, 0.17018379850238258, 0.17487330761667044, 0.17532713107934347, 0.17956281673095834, 0.18001664019363134, 0.18629453142727478, 0.1962030103623024, 0.20180016640193632, 0.20573330307843582, 0.20981771424249301, 0.2151879585507904, 0.22850011345586566, 0.23243325013236518, 0.23636638680886468, 0.2424173663111716, 0.2491490810074881, 0.2558051584600257, 0.262612510400121, 0.2685122154148703, 0.2838665759019741, 0.2883291732849255, 0.3007336812646547, 0.3048180924287119, 0.31109598366235536, 0.3120792678314802, 0.31654186521443156, 0.31790333560245065, 0.325164511005219, 0.33098857877618937, 0.3355268134029196, 0.3390817638605249, 0.344149459193707, 0.34906588003933137, 0.35330156569094623, 0.35731033961122455, 0.35814234929279176, 0.3585961727554648, 0.3590499962181378, 0.363134407382195, 0.36389077981998336, 0.3690341123969443, 0.3806822479388851, 0.3849935708342788, 0.40239013690341124, 0.4031465093411996, 0.4072309205052568, 0.4123742530822177, 0.4207699871416686, 0.42795552530065806, 0.43249375992738825, 0.4386960139172529, 0.4421753271310793, 0.4473942969518191, 0.4538234626730202, 0.46017699115044247, 0.46713561757809546, 0.46789199001588383, 0.4725814991301717, 0.48090159594584375, 0.48687693820437183, 0.49035625141819833, 0.4972392406020725, 0.4976930640647455, 0.5041978670297255, 0.5108539444822631, 0.5148627184025414, 0.5271159518947129, 0.5316541865214431, 0.5380833522426443, 0.5445881552076243, 0.5486725663716814, 0.5520006050979502, 0.5524544285606232, 0.5570683004311323, 0.5575221238938053, 0.5622116330080932, 0.5626654564707662, 0.5631192799334392, 0.5762801603509569, 0.5822555026094849, 0.5830875122910522, 0.5889872173058014, 0.5955676575145602, 0.601240450797973, 0.6127373118523561, 0.6134936842901445, 0.6258981922698736, 0.6296800544588155, 0.6392859844187277, 0.6448075032145829, 0.6528250510551395, 0.657287648438091, 0.6577414719007639, 0.6668179411542243, 0.6672717646168974, 0.6687845094924741, 0.6737765675818773, 0.6780122532334921, 0.6836850465169049, 0.684138869979578, 0.688752741850087, 0.6920807805763558, 0.6972997503970956, 0.6978292111035473, 0.7023674457302775, 0.7062249451629983, 0.7073595038196808, 0.7115195522275168, 0.7169654337795931, 0.7230920505256788, 0.7298237652219953, 0.7435897435897436, 0.7473716057786854, 0.7519098404054156, 0.7523636638680886, 0.7593979275395205, 0.764238711141366, 0.7680205733303078, 0.774903562514182, 0.7796687088722487, 0.8009227743741018, 0.8064442931699569, 0.8110581650404659, 0.8118901747220332, 0.8250510551395507, 0.825807427577339, 0.8301187504727328, 0.8348082595870207, 0.8404054156266546, 0.8452461992285001, 0.8460025716662884, 0.8524317373874896, 0.8574994327206716, 0.8615838438847289, 0.8711897738446411, 0.8719461462824295, 0.8767112926404962, 0.8822328114363512, 0.8889645261326677, 0.8949398683911958, 0.9005370244308297, 0.9056803570077906, 0.9116556992663187, 0.9272369714847591, 0.9309431964299221, 0.9361621662506618, 0.9484153997428334, 0.9540125557824672, 0.9579456924589668, 0.9680054458815521, 0.9808637773239544, 0.985628923682021, 0.9907722562589819, 0.9956886771046063, 1}

// Generator produces lorem ipsum text. It is not safe for concurrent use.
type Generator struct {
	rng    *rand.Rand
	opened bool
}

// NewGenerator returns a Generator seeded at random, so two of them produce
// different text.
func NewGenerator() *Generator {
	var seed [32]byte
	for i := range seed {
		seed[i] = rand.N[byte](255)
	}
	return &Generator{
		rng: rand.New(rand.NewChaCha8(seed)),
	}
}

// Words returns that many words of lorem ipsum. The first call opens with the
// conventional "Lorem ipsum dolor sit amet…"; later calls carry on with random
// words.
func (g *Generator) Words(words int) string {
	opening := !g.opened
	g.opened = true
	return generate(g.rng, opening, words)
}

func generate(rng *rand.Rand, withOpening bool, words int) string {
	if words < 1 {
		return ""
	}
	var sb strings.Builder
	if withOpening {
		openingLimit := min(words, 18)
		// If the requested word count is close to the opening text length,
		// adjust the opening limit to avoid a dangling tiny tail.
		if words > openingLimit && words-openingLimit < 4 {
			openingLimit -= 5
		}
		words -= opening(&sb, openingLimit)
	}

	for words > 0 {
		sentenceLen := min(binomial(rng, 46, 0.36)+4, words)
		words -= sentenceLen

		firstWordInSentence := true
		for sentenceLen > 0 {
			clauseLen := min(binomial(rng, 12, 0.45)+3, sentenceLen)
			// Prevent dangling tiny clauses (e.g., 1-3 words left over) by
			// absorbing them into the current clause.
			if sentenceLen-clauseLen > 0 && sentenceLen-clauseLen < 4 {
				clauseLen = sentenceLen
			}
			for range clauseLen {
				if sb.Len() > 0 {
					sb.WriteByte(' ')
				}
				word := generateWord(rng)
				if firstWordInSentence {
					first, width := utf8.DecodeRuneInString(word)
					word = string(unicode.ToUpper(first)) + word[width:]
					firstWordInSentence = false
				}
				sb.WriteString(word)
			}
			sentenceLen -= clauseLen
			if sentenceLen > 0 {
				sb.WriteByte(',')
			} else {
				sb.WriteByte('.')
			}
		}
	}

	return sb.String()
}

func opening(sb *strings.Builder, limit int) int {
	offset := 0
	words := 0
	for i, r := range openingText {
		offset = i
		if r == ' ' {
			words++
			if words >= limit {
				break
			}
		}
	}
	if openingText[offset-1] == ',' {
		offset--
	}
	sb.WriteString(openingText[:offset])
	if words >= 5 && openingText[offset-1] != '.' {
		sb.WriteByte('.')
	}
	return words
}

func binomial(rng *rand.Rand, n int, p float64) int {
	x := 0
	for range n {
		if rng.Float64() < p {
			x++
		}
	}
	return x
}

func generateWord(rng *rand.Rand) string {
	r := rng.Float64()
	idx, _ := slices.BinarySearch(cumProp, r)
	return words[idx]
}
