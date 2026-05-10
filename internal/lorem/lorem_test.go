package lorem

import (
	"crypto/sha256"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var testSeed = sha256.Sum256([]byte(openingText))

func TestGenerate(t *testing.T) {
	tests := []struct {
		words int
		want  string
	}{
		{0, ""},
		{1, "Lorem"},
		{2, "Lorem ipsum"},
		{3, "Lorem ipsum dolor"},
		{4, "Lorem ipsum dolor sit"},
		{5, "Lorem ipsum dolor sit amet."},
		{6, "Lorem ipsum dolor sit amet, consectetur."},
		{7, "Lorem ipsum dolor sit amet, consectetur adipiscing."},
		{18, "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna."},
		{19, "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt. Erat sodales euismod suscipit accumsan nulla."},
		{20, "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt. Erat sodales euismod suscipit accumsan nulla fringilla."},
		{21, "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt. Erat sodales euismod suscipit accumsan nulla fringilla pellentesque."},
		{30, "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna. Erat sodales euismod suscipit accumsan nulla fringilla pellentesque, imperdiet tempor felis dignissim."},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d_words", tt.words), func(t *testing.T) {
			gen := &Generator{
				rng: rand.New(rand.NewChaCha8(testSeed)),
			}
			if got := gen.Words(tt.words); got != tt.want {
				t.Errorf("Generate(%d) = %q, want %q", tt.words, got, tt.want)
			}
		})
	}
}

var largeText = `Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna. Erat sodales euismod suscipit accumsan nulla fringilla pellentesque, imperdiet tempor felis dignissim nisi magna arcu augue convallis suspendisse. Hendrerit bibendum felis aliquet erat egestas vel bibendum nisi proin, amet libero ligula ut dictum etiam elit dignissim. Ipsum auctor dui morbi sagittis blandit, posuere libero tellus eu eu et dictum, et erat sagittis consequat ligula non suspendisse. Ligula blandit sed tristique leo aliquam, posuere ex orci eros viverra nulla ullamcorper interdum sem, iaculis senectus amet accumsan id turpis viverra pretium. Porta ullamcorper orci.
Orci luctus porta sem turpis sed ac aliquam at maximus, dictum consectetur feugiat nec ut neque arcu auctor. Venenatis orci vitae tortor vulputate vestibulum justo, consectetur id commodo felis tortor amet nisl eget tincidunt lectus nec. Cubilia at quam id mauris vel ut quis quis, proin ut proin ut sollicitudin dapibus, suspendisse sollicitudin leo pulvinar lacus semper donec ultricies a. At scelerisque in suscipit suspendisse id ligula per viverra, id nulla faucibus at dictum in libero quam auctor sit. Magna sed nisl odio lectus aenean pellentesque justo eros, vehicula cras sed habitant nunc et eu in vel viverra pellentesque nulla.
Mi dignissim feugiat erat purus aliquet conubia commodo magna, tellus luctus arcu sapien metus eget turpis interdum, quam sed ultrices eget orci. Eget mi hac vehicula etiam aliquet interdum duis laoreet, libero sagittis donec elementum donec dapibus tellus, tincidunt lorem mauris maecenas arcu. Tempor ut posuere lacinia donec luctus ex erat est, consectetur tincidunt fusce at suspendisse bibendum et iaculis, vivamus dapibus donec ultricies a mi. Ut nullam dapibus justo nunc dolor vitae leo ut, ante vestibulum sed velit neque sed phasellus. Eleifend massa tellus non ullamcorper diam aenean amet malesuada non pellentesque, platea sed sit dapibus sed efficitur purus.
Et non a eu risus ipsum non, nisi sodales tempor ullamcorper facilisis tortor tincidunt ac, vestibulum quis metus lacus dolor bibendum augue etiam dignissim vulputate. Interdum rutrum orci etiam non ut sollicitudin at purus, a blandit molestie eget quam luctus et leo. At quis fermentum quis donec curae nam nunc dui rutrum, dui morbi lorem vestibulum faucibus sem et, donec nisl nunc enim vel quam feugiat. Fermentum neque orci in molestie vestibulum luctus efficitur, magna nulla commodo enim commodo. Ex egestas praesent nulla amet a vulputate ultricies, condimentum consequat vel penatibus imperdiet sit hendrerit eget a tempor tempor. Sit rutrum.
Enim venenatis in tempor rhoncus mauris vehicula neque, diam aliquam luctus tincidunt pulvinar nibh a, nulla sodales posuere ligula mattis quisque ligula, amet diam condimentum blandit. Viverra dictum dapibus tristique mauris condimentum imperdiet vitae, quis magna primis class elit sed praesent erat amet, nulla lobortis orci enim ex in. Quis tincidunt egestas sapien eget placerat pellentesque elementum purus morbi, ligula bibendum nisl elementum velit vel lacinia curabitur justo accumsan dictum justo, faucibus euismod viverra ultricies condimentum. Amet dui eget mi augue velit et ultrices nec tellus, auctor rhoncus mauris elit efficitur. Nunc venenatis quis donec cras malesuada id morbi ut.
Feugiat porta quis porta ac ullamcorper erat ligula facilisi justo lobortis augue, curabitur ultrices nibh massa per etiam, orci fusce volutpat lacinia risus ut ultrices cursus pellentesque. Fringilla aliquet nisl fusce in massa lacus dolor, in nullam vestibulum quam tincidunt nec mi in lobortis laoreet laoreet porta. In hendrerit suspendisse senectus turpis at vehicula cras aenean, purus sed ullamcorper ex nisl ac consectetur tristique volutpat, et nisl dui lectus ipsum efficitur. Euismod nisl sed ridiculus sed nisi felis, condimentum magna et sed turpis id, et libero arcu eget sed. Bibendum id purus vulputate eget sagittis blandit nisl ultrices nunc per.
Vehicula magna quis pulvinar eleifend tellus egestas nisi aliquam, quis sed luctus lorem quam ac mattis, sollicitudin leo at fringilla lobortis nec. Vitae nibh vel luctus litora ornare nisi sit ut, ante tellus placerat orci fringilla ultrices at finibus faucibus elit scelerisque volutpat, viverra lorem odio mollis suscipit luctus. Viverra eget neque augue at sagittis eget et, class mollis consequat suscipit vivamus libero laoreet accumsan accumsan non. Malesuada etiam nunc id felis auctor vestibulum maximus elementum, rutrum quam fringilla quis arcu eu. Eget at aliquam eget nulla consequat eget consectetur, quam vitae arcu dui rutrum lorem nunc ipsum lorem blandit.
Eu diam ante ac laoreet turpis eget fusce volutpat ante, porttitor posuere tellus bibendum at fames sit auctor gravida. Nullam eleifend aenean ultrices commodo arcu sed magna, dolor vitae at egestas et accumsan duis euismod sollicitudin elementum, ligula turpis leo sit lorem aliquam vitae. Mattis ut ipsum vestibulum condimentum blandit ut viverra ut, at cursus nunc fusce sed lacus iaculis nunc justo primis viverra odio. Molestie orci rutrum morbi nec velit nunc praesent sed lectus, aenean augue venenatis et pharetra vestibulum maximus aliquam ligula pellentesque quam vitae. Ante non vel auctor orci scelerisque sed volutpat nulla etiam facilisi eget donec.
Metus mi maximus nec tincidunt felis risus mauris, ut metus orci nulla ultricies fringilla ipsum neque et dictum. Volutpat purus ut erat pellentesque facilisis donec quis arcu, commodo enim lorem blandit nibh convallis justo in vel. Euismod sodales porta efficitur ultricies a velit lacinia quis eleifend vivamus, himenaeos sed bibendum nec sagittis et scelerisque massa nisl suspendisse eget. Id lectus vestibulum eros at leo ac proin orci, aliquam ex dolor consectetur interdum posuere pretium dictum. Vestibulum semper faucibus lorem dictum vivamus orci et, arcu id quis sapien dapibus metus, massa in nisi orci venenatis, ante mollis orci eu elementum faucibus.
Dui congue nisi vestibulum at ullamcorper quam fermentum, pellentesque mollis id lorem ut porta, mauris vulputate proin commodo vestibulum et. At in laoreet ipsum ligula risus vitae justo, justo molestie vivamus vitae ligula lorem ac a enim in non aenean. Vitae faucibus donec consequat at volutpat ultrices per, elit maximus magna tempor a vel sem nam fermentum ac eu. Nulla consequat tempor mauris volutpat tempus pretium velit, orci ornare varius malesuada eget amet in sagittis amet, ac vivamus morbi praesent efficitur. Ac facilisis eget eu lacinia lobortis per efficitur nunc ultricies vulputate tempor, lacus est lacinia nisl blandit porttitor commodo.
Mattis nec sit ante suscipit non eget porta, amet purus a sagittis nulla ornare fringilla non in. Eget malesuada feugiat purus enim non neque nunc, faucibus pulvinar venenatis faucibus quisque sit libero, quisque posuere mi vivamus quisque dignissim at ut. In proin curabitur viverra elit ante bibendum himenaeos quam, mi nunc dapibus vel sed luctus enim curabitur, aenean eget erat elit. Id placerat ligula laoreet pharetra sapien duis efficitur, at quis et vulputate metus mi at ornare ornare, finibus et nulla proin vestibulum. Risus id erat non malesuada pharetra ultricies ligula, magna convallis mauris id cursus ornare vel enim semper.
Metus nulla tellus magna elementum vestibulum ornare vehicula mollis sed, tincidunt accumsan facilisis vel lacus donec magna quis, dui lobortis massa pellentesque. Morbi auctor velit bibendum purus nec elit aliquam a amet id, rhoncus in rhoncus suscipit pulvinar in maximus tellus. Fames turpis malesuada consequat nisi eleifend vitae, eros sed ultricies nunc posuere interdum vitae risus amet nisi non vel a. Odio dignissim scelerisque in aliquam suscipit rhoncus vehicula, ac augue pretium odio nunc at fusce in, id venenatis sed posuere mauris lacus. Velit congue sit volutpat consectetur nec porta est pretium eget sit, diam aliquam volutpat volutpat lectus in.
Porttitor eget tincidunt justo tellus eget vehicula nec fusce dignissim, sociosqu eu ornare suscipit phasellus eu commodo ipsum, ultrices rutrum id porta semper odio nibh morbi tortor. Faucibus scelerisque nascetur dolor rutrum aliquam velit, odio sit erat malesuada tortor ultricies metus turpis luctus, nibh nisi orci nisi aliquam nam habitasse praesent. Turpis id sodales ultricies aliquet viverra amet turpis ac finibus, amet scelerisque urna lacus ut. Dolor purus a in amet commodo, quisque felis malesuada purus ipsum rhoncus venenatis ipsum auctor, scelerisque quis ut sed lobortis lobortis eget. Tincidunt mi tortor ipsum enim nisi eros justo auctor montes lectus curabitur.
Fringilla lacinia nec id scelerisque lacus in mi ut tristique lacus, justo egestas sed dignissim eros proin et. Sed phasellus ante nam massa ultrices ultrices id nunc dui, iaculis risus porta sed himenaeos vel semper fringilla malesuada. Ut faucibus luctus rutrum ac lacus aliquam, augue quis eget ligula in magna a vestibulum, purus ut neque duis semper. Enim nulla tristique ex odio erat lectus lorem quis aenean, fusce libero ac venenatis nam porttitor hendrerit commodo integer nulla ligula. Condimentum integer pharetra habitant nulla vel pellentesque imperdiet gravida suspendisse, varius tincidunt sollicitudin lorem eros tincidunt laoreet, suspendisse interdum mi tincidunt tincidunt.
Amet ad nullam risus risus urna curabitur nisi curabitur, et malesuada nec nisl bibendum fringilla dolor justo aliquam quis eu. Ullamcorper augue et suspendisse orci mi ultricies ante elit cras molestie fusce, dui tortor euismod bibendum. Tincidunt et diam aenean tortor efficitur ultrices blandit, sem sollicitudin luctus dolor et sed ipsum suspendisse cubilia velit sed nulla. Sapien sed mattis aenean sem at in lacinia faucibus luctus lorem quam dictum, in gravida condimentum dignissim dui risus metus dictum. Vestibulum euismod ut eget metus vitae nulla molestie euismod quam, nam diam ante tristique et sollicitudin eros tincidunt sit nibh. Faucibus ullamcorper libero.
`

func TestGenerateLarge(t *testing.T) {
	gen := &Generator{
		rng: rand.New(rand.NewChaCha8(testSeed)),
	}
	var sb strings.Builder
	for range 15 {
		sb.WriteString(gen.Words(100))
		sb.WriteByte('\n')
	}
	got := sb.String()
	if diff := cmp.Diff(largeText, got); diff != "" {
		t.Errorf("Generate(1500) mismatch (-want +got):\n%s", diff)
	}
}
