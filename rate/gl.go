//go:build !nogl
// +build !nogl

package rate

import (
	"fmt"
	"image"
	"strings"

	"github.com/go-gl/gl/v4.6-core/gl"
)

func imgTexture(img *image.RGBA) (uint32, error) {
	b := img.Bounds()
	var texture uint32
	gl.GenTextures(1, &texture)
	gl.BindTexture(gl.TEXTURE_2D, texture)
	const m = gl.NEAREST
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, m)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, m)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(
		gl.TEXTURE_2D,
		0,
		gl.RGBA,
		int32(b.Dx()),
		int32(b.Dy()),
		0,
		gl.RGBA,
		gl.UNSIGNED_BYTE,
		gl.Ptr(img.Pix),
	)

	return texture, nil
}

func releaseTexture(tex uint32) error {
	gl.DeleteTextures(1, &tex)
	return nil
}

func compileShader(source string, shaderType uint32) (uint32, error) {
	shader := gl.CreateShader(shaderType)

	csources, free := gl.Strs(source)
	l := int32(len(source))
	gl.ShaderSource(shader, 1, csources, &l)
	free()
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)

		log := strings.Repeat("\x00", int(logLength+1))
		gl.GetShaderInfoLog(shader, logLength, nil, gl.Str(log))

		return 0, fmt.Errorf("failed to compile %v: %v", source, log)
	}

	return shader, nil
}

func newProgram() (uint32, error) {
	vertexShaderSrc := `#version 410 core
layout (location = 0) in vec2 pos;
layout (location = 1) in vec2 tex;
out vec2 TexCoord;
out vec3 FragPos;

uniform mat4 projection;
uniform mat4 model;

void main()
{
    FragPos = vec3(model * vec4(pos, 0.0, 1.0));
    gl_Position = projection * model * vec4(pos, 0.0, 1.0);
    TexCoord = tex;
}`

	fragShaderSrc := `#version 410 core
out vec4 color;
in vec2 TexCoord;
in vec3 FragPos;

uniform sampler2D texture1;
uniform mat4 projection;
uniform int invert;

void main()
{
    color = texture(texture1, TexCoord);
	if (invert == 1)
		color = vec4(1.0 - color.r, 1.0 - color.g, 1.0 - color.b, color.a);
	if (color.a <= 0.02)
        discard;
}`
	vertexShader, err := compileShader(vertexShaderSrc, gl.VERTEX_SHADER)
	if err != nil {
		return 0, err
	}

	fragmentShader, err := compileShader(fragShaderSrc, gl.FRAGMENT_SHADER)
	if err != nil {
		return 0, err
	}

	program := gl.CreateProgram()
	gl.AttachShader(program, vertexShader)
	gl.AttachShader(program, fragmentShader)
	gl.LinkProgram(program)

	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLength)
		log := strings.Repeat("\x00", int(logLength+1))
		gl.GetProgramInfoLog(program, logLength, nil, gl.Str(log))
		return 0, fmt.Errorf("failed to link program: %v", log)
	}

	gl.DeleteShader(vertexShader)
	gl.DeleteShader(fragmentShader)
	return program, nil
}
