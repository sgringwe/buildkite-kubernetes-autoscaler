# iron/go is the alpine image with only ca-certificates added
FROM iron/go

# Now just add the binary
ADD bin/buildkite-kubernetes-autoscaler /

ENTRYPOINT ["./buildkite-kubernetes-autoscaler"]