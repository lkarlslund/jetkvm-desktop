package gtkui

/*
#cgo pkg-config: epoxy
#include <epoxy/gl.h>
#include <stdio.h>
#include <stdlib.h>

// --- Thin wrappers so CGo can call epoxy-resolved GL functions ---

static GLuint compileShader(GLenum kind, const char *src) {
	GLuint s = glCreateShader(kind);
	glShaderSource(s, 1, &src, NULL);
	glCompileShader(s);
	GLint ok = 0;
	glGetShaderiv(s, GL_COMPILE_STATUS, &ok);
	if (!ok) {
		char log[512];
		glGetShaderInfoLog(s, sizeof(log), NULL, log);
		fprintf(stderr, "shader compile: %s\n", log);
		glDeleteShader(s);
		return 0;
	}
	return s;
}

static GLuint linkProgram(GLuint vs, GLuint fs) {
	GLuint p = glCreateProgram();
	glAttachShader(p, vs);
	glAttachShader(p, fs);
	glLinkProgram(p);
	GLint ok = 0;
	glGetProgramiv(p, GL_LINK_STATUS, &ok);
	if (!ok) {
		char log[512];
		glGetProgramInfoLog(p, sizeof(log), NULL, log);
		fprintf(stderr, "program link: %s\n", log);
		glDeleteProgram(p);
		return 0;
	}
	return p;
}

static GLuint buildProgram(const char *vsSrc, const char *fsSrc) {
	GLuint vs = compileShader(GL_VERTEX_SHADER, vsSrc);
	if (!vs) return 0;
	GLuint fs = compileShader(GL_FRAGMENT_SHADER, fsSrc);
	if (!fs) { glDeleteShader(vs); return 0; }
	GLuint p = linkProgram(vs, fs);
	glDeleteShader(vs);
	glDeleteShader(fs);
	return p;
}

static void setupQuad(GLuint *vao, GLuint *vbo) {
	float verts[] = {
		-1, -1,   0, 1,
		 1, -1,   1, 1,
		-1,  1,   0, 0,
		 1,  1,   1, 0,
	};
	glGenVertexArrays(1, vao);
	glGenBuffers(1, vbo);
	glBindVertexArray(*vao);
	glBindBuffer(GL_ARRAY_BUFFER, *vbo);
	glBufferData(GL_ARRAY_BUFFER, sizeof(verts), verts, GL_STATIC_DRAW);
	glVertexAttribPointer(0, 2, GL_FLOAT, GL_FALSE, 4*sizeof(float), (void*)0);
	glEnableVertexAttribArray(0);
	glVertexAttribPointer(1, 2, GL_FLOAT, GL_FALSE, 4*sizeof(float), (void*)(2*sizeof(float)));
	glEnableVertexAttribArray(1);
	glBindVertexArray(0);
}

static void initTexture(GLuint *tex) {
	glGenTextures(1, tex);
	glBindTexture(GL_TEXTURE_2D, *tex);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);
	glBindTexture(GL_TEXTURE_2D, 0);
}

static void uploadRGBA(GLuint tex, int w, int h, int allocW, int allocH, const void *pix) {
	glBindTexture(GL_TEXTURE_2D, tex);
	if (w != allocW || h != allocH) {
		glTexImage2D(GL_TEXTURE_2D, 0, GL_RGBA8, w, h, 0,
			GL_RGBA, GL_UNSIGNED_BYTE, pix);
	} else {
		glTexSubImage2D(GL_TEXTURE_2D, 0, 0, 0, w, h,
			GL_RGBA, GL_UNSIGNED_BYTE, pix);
	}
	glBindTexture(GL_TEXTURE_2D, 0);
}

static void drawFrame(GLuint program, GLuint vao, GLuint tex,
                       int viewW, int viewH, int texW, int texH) {
	glClearColor(0.1f, 0.1f, 0.1f, 1.0f);
	glClear(GL_COLOR_BUFFER_BIT);

	if (texW == 0 || texH == 0) return;

	// Letterbox viewport to preserve source aspect ratio.
	float srcA = (float)texW / (float)texH;
	float dstA = (float)viewW / (float)viewH;
	int vpX, vpY, vpW, vpH;
	if (srcA > dstA) {
		vpW = viewW;
		vpH = (int)((float)viewW / srcA);
		vpX = 0;
		vpY = (viewH - vpH) / 2;
	} else {
		vpH = viewH;
		vpW = (int)((float)viewH * srcA);
		vpX = (viewW - vpW) / 2;
		vpY = 0;
	}
	glViewport(vpX, vpY, vpW, vpH);

	glUseProgram(program);
	glActiveTexture(GL_TEXTURE0);
	glBindTexture(GL_TEXTURE_2D, tex);

	GLint loc = glGetUniformLocation(program, "tex");
	glUniform1i(loc, 0);

	glBindVertexArray(vao);
	glDrawArrays(GL_TRIANGLE_STRIP, 0, 4);
	glBindVertexArray(0);

	glBindTexture(GL_TEXTURE_2D, 0);
	glUseProgram(0);

	// Restore full viewport for GTK compositing.
	glViewport(0, 0, viewW, viewH);
}

static void cleanupGL(GLuint tex, GLuint vbo, GLuint vao,
                       GLuint prog1, GLuint prog2) {
	if (tex)   glDeleteTextures(1, &tex);
	if (vbo)   glDeleteBuffers(1, &vbo);
	if (vao)   glDeleteVertexArrays(1, &vao);
	if (prog1) glDeleteProgram(prog1);
	if (prog2) glDeleteProgram(prog2);
}
*/
import "C"
import "unsafe"

type glState struct {
	rgbaProgram  C.GLuint
	ycbcrProgram C.GLuint
	vao          C.GLuint
	vbo          C.GLuint
	texture      C.GLuint
	texW, texH   int
}

const vertexShaderSrc = `#version 300 es
precision mediump float;
layout(location = 0) in vec2 position;
layout(location = 1) in vec2 texCoord;
out vec2 vTexCoord;
void main() {
    gl_Position = vec4(position, 0.0, 1.0);
    vTexCoord = texCoord;
}
`

const rgbaFragmentSrc = `#version 300 es
precision mediump float;
in vec2 vTexCoord;
out vec4 fragColor;
uniform sampler2D tex;
void main() {
    fragColor = texture(tex, vTexCoord);
}
`

const ycbcrFragmentSrc = `#version 300 es
precision mediump float;
in vec2 vTexCoord;
out vec4 fragColor;
uniform sampler2D tex;
void main() {
    vec4 p = texture(tex, vTexCoord);
    float y  = p.r;
    float cb = p.g - 0.5;
    float cr = p.b - 0.5;
    fragColor = vec4(
        y + 1.402 * cr,
        y - 0.344136 * cb - 0.714136 * cr,
        y + 1.772 * cb,
        1.0
    );
}
`

func glInit(st *glState) bool {
	vs := C.CString(vertexShaderSrc)
	defer C.free(unsafe.Pointer(vs))
	rgbaFS := C.CString(rgbaFragmentSrc)
	defer C.free(unsafe.Pointer(rgbaFS))
	ycbcrFS := C.CString(ycbcrFragmentSrc)
	defer C.free(unsafe.Pointer(ycbcrFS))

	st.rgbaProgram = C.buildProgram(vs, rgbaFS)
	st.ycbcrProgram = C.buildProgram(vs, ycbcrFS)
	if st.rgbaProgram == 0 || st.ycbcrProgram == 0 {
		return false
	}

	C.setupQuad(&st.vao, &st.vbo)
	C.initTexture(&st.texture)
	return true
}

func glUploadRGBA(st *glState, w, h int, pix []byte) {
	C.uploadRGBA(st.texture, C.int(w), C.int(h),
		C.int(st.texW), C.int(st.texH), unsafe.Pointer(&pix[0]))
	st.texW = w
	st.texH = h
}

func glDraw(st *glState, program C.GLuint, viewW, viewH, texW, texH int) {
	C.drawFrame(program, st.vao, st.texture,
		C.int(viewW), C.int(viewH), C.int(texW), C.int(texH))
}

func glCleanup(st *glState) {
	C.cleanupGL(st.texture, st.vbo, st.vao,
		st.rgbaProgram, st.ycbcrProgram)
	*st = glState{}
}
