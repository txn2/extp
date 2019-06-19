FROM scratch
ENV PATH=/bin

COPY extp /bin/

WORKDIR /

ENTRYPOINT ["/bin/extp"]