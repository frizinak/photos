module github.com/frizinak/photos

go 1.14

replace github.com/dsoprea/go-exif/v3 v3.0.0-20210625224831-a6301f85c82b => github.com/frizinak/go-exif/v3 v3.0.0-20220519145717-3b938bf2a729

require (
	github.com/dsoprea/go-exif/v3 v3.0.0-20210625224831-a6301f85c82b
	github.com/dsoprea/go-jpeg-image-structure/v2 v2.0.0-20210512043942-b434301c6836
	github.com/frizinak/gphoto2go v0.0.0-20200727103018-6698a73f379d
	github.com/go-gl/gl v0.0.0-20190320180904-bf2b1f2f34d7
	github.com/go-gl/glfw/v3.3/glfw v0.0.0-20200625191551-73d3c3675aa3
	github.com/go-gl/mathgl v0.0.0-20190713194549-592312d8590a
	github.com/json-iterator/go v1.1.12
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	golang.org/x/image v0.0.0-20220413100746-70e8d0d3baa9 // indirect
	gopkg.in/ini.v1 v1.66.4
)
