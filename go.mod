module github.com/frizinak/photos

go 1.14

replace github.com/dsoprea/go-exif/v3 v3.0.0-20210625224831-a6301f85c82b => github.com/frizinak/go-exif/v3 v3.0.0-20220519145717-3b938bf2a729

require (
	github.com/dsoprea/go-exif/v3 v3.0.0-20210625224831-a6301f85c82b
	github.com/dsoprea/go-jpeg-image-structure/v2 v2.0.0-20210512043942-b434301c6836
	github.com/frizinak/gphoto2go v0.0.0-20200727103018-6698a73f379d
	github.com/frizinak/phodo v0.0.0-20230726095614-9a9aa47f89a0 // indirect
	github.com/go-gl/gl v0.0.0-20211210172815-726fda9656d6
	github.com/go-gl/glfw/v3.3/glfw v0.0.0-20221017161538-93cebf72946b
	github.com/go-gl/mathgl v1.0.0
	github.com/json-iterator/go v1.1.12
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	golang.org/x/image v0.8.0
	gopkg.in/ini.v1 v1.66.4
)
