module github.com/corygyarmathy/matrix-display

// rpi01 builds the renderer with the Go from nixos-26.05 (the overlay uses the
// consumer's toolchain), so this must not ask for a newer one.
go 1.26
