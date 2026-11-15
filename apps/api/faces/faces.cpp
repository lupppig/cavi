// cavi-faces finds faces in video frames for the media worker's framing
// analysis, with OpenCV's YuNet detector (MIT licence, see LICENSE.yunet).
//
// Usage: cavi-faces MODEL WIDTH HEIGHT
//
// It reads raw BGR frames (WIDTH*HEIGHT*3 bytes each) from stdin and, for
// each, writes one line to stdout: the number of faces, then each face's
// box and confidence in pixels: "n x y w h score x y w h score ...". It
// reads nothing else and opens no files besides MODEL.
#include <cstdio>
#include <cstdlib>
#include <vector>

#include <opencv2/core.hpp>
#include <opencv2/objdetect/face.hpp>

int main(int argc, char** argv) {
    if (argc != 4) {
        std::fprintf(stderr, "usage: cavi-faces MODEL WIDTH HEIGHT\n");
        return 2;
    }
    const int w = std::atoi(argv[2]), h = std::atoi(argv[3]);
    if (w < 16 || h < 16 || w > 4096 || h > 4096) {
        std::fprintf(stderr, "cavi-faces: invalid frame size\n");
        return 2;
    }
    // The worker runs several jobs at once: stay off most cores.
    cv::setNumThreads(2);
    cv::Ptr<cv::FaceDetectorYN> det;
    try {
        // Score threshold 0.6, NMS 0.3, at most 50 faces a frame.
        det = cv::FaceDetectorYN::create(argv[1], "", cv::Size(w, h), 0.6f, 0.3f, 50);
    } catch (const cv::Exception& e) {
        std::fprintf(stderr, "cavi-faces: load model: %s\n", e.what());
        return 1;
    }
    std::vector<unsigned char> buf(static_cast<size_t>(w) * h * 3);
    while (std::fread(buf.data(), 1, buf.size(), stdin) == buf.size()) {
        cv::Mat frame(h, w, CV_8UC3, buf.data());
        cv::Mat faces;
        try {
            det->detect(frame, faces);
        } catch (const cv::Exception& e) {
            std::fprintf(stderr, "cavi-faces: detect: %s\n", e.what());
            return 1;
        }
        std::printf("%d", faces.rows);
        for (int i = 0; i < faces.rows; i++) {
            const float* f = faces.ptr<float>(i);
            // Columns: box (4), five landmarks (10), score.
            std::printf(" %.1f %.1f %.1f %.1f %.3f", f[0], f[1], f[2], f[3], f[14]);
        }
        std::printf("\n");
        std::fflush(stdout);
    }
    return 0;
}
